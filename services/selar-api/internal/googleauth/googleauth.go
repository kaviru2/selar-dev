// Package googleauth implements the server side of "Sign in with Google":
// exchanging an authorization code (with its PKCE verifier) at Google's token
// endpoint and verifying the returned OpenID Connect ID token.
//
// Only the standard library is used. The ID token is verified locally:
// RS256 signature against Google's published JWKS, then iss, aud, azp, exp,
// iat, nonce, sub and email_verified. The access token Google also returns
// is discarded; SELAR requests only the "openid email" scopes.
package googleauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultTokenURL is Google's OAuth 2.0 token endpoint.
	DefaultTokenURL = "https://oauth2.googleapis.com/token"
	// DefaultJWKSURL publishes the keys that sign Google ID tokens.
	DefaultJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"

	clockSkew        = 2 * time.Minute
	defaultKeysTTL   = time.Hour
	minRefetchPeriod = time.Minute
	maxBody          = 1 << 20
)

// Google issues ID tokens with either form of the issuer.
var validIssuers = map[string]bool{"https://accounts.google.com": true, "accounts.google.com": true}

// Errors returned by Verify. ErrEmailNotVerified is distinguished so the
// handler can explain why sign-in was refused.
var (
	ErrInvalidToken     = errors.New("invalid Google ID token")
	ErrEmailNotVerified = errors.New("Google account email is not verified")
	ErrNotConfigured    = errors.New("Google sign-in is not configured")
)

// Identity is the verified subset of the ID token SELAR uses. Name and
// picture are deliberately not read: the "profile" scope is not requested.
type Identity struct {
	Subject string // Google's stable account id ("sub")
	Email   string
}

// Client exchanges codes and verifies ID tokens for one OAuth client.
type Client struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
	JWKSURL      string
	HTTPClient   *http.Client
	Now          func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	keysUntil time.Time
	lastFetch time.Time
}

// FromEnv builds a client from GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET.
// It returns nil when either is unset, which disables Google sign-in.
func FromEnv(getenv func(string) string) *Client {
	id := strings.TrimSpace(getenv("GOOGLE_CLIENT_ID"))
	secret := strings.TrimSpace(getenv("GOOGLE_CLIENT_SECRET"))
	if id == "" || secret == "" {
		return nil
	}
	return &Client{ClientID: id, ClientSecret: secret}
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func orDefault(v, d string) string {
	if v != "" {
		return v
	}
	return d
}

// Exchange redeems an authorization code and returns the verified identity.
func (c *Client) Exchange(ctx context.Context, code, codeVerifier, redirectURI, nonce string) (*Identity, error) {
	if c == nil || c.ClientID == "" || c.ClientSecret == "" {
		return nil, ErrNotConfigured
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"redirect_uri":  {redirectURI},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(c.TokenURL, DefaultTokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := c.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		// Only Google's error code is surfaced (never the code or secret).
		return nil, fmt.Errorf("token exchange: status %d %s", res.StatusCode, e.Error)
	}
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.IDToken == "" {
		return nil, fmt.Errorf("token exchange: no id_token in response")
	}
	return c.Verify(ctx, tok.IDToken, nonce)
}

type idClaims struct {
	Iss           string          `json:"iss"`
	Aud           json.RawMessage `json:"aud"`
	Azp           string          `json:"azp"`
	Sub           string          `json:"sub"`
	Email         string          `json:"email"`
	EmailVerified json.RawMessage `json:"email_verified"`
	Nonce         string          `json:"nonce"`
	Exp           int64           `json:"exp"`
	Iat           int64           `json:"iat"`
}

func (c idClaims) audiences() []string {
	var one string
	if json.Unmarshal(c.Aud, &one) == nil {
		return []string{one}
	}
	var many []string
	_ = json.Unmarshal(c.Aud, &many)
	return many
}

// emailVerified accepts the boolean Google sends (and the string form some
// libraries historically emitted).
func (c idClaims) emailVerified() bool {
	var b bool
	if json.Unmarshal(c.EmailVerified, &b) == nil {
		return b
	}
	var s string
	return json.Unmarshal(c.EmailVerified, &s) == nil && s == "true"
}

// Verify checks an ID token's signature and claims. expectedNonce must be the
// nonce SELAR put in the authorization request; an empty nonce is refused.
func (c *Client) Verify(ctx context.Context, idToken, expectedNonce string) (*Identity, error) {
	if c == nil || c.ClientID == "" {
		return nil, ErrNotConfigured
	}
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: malformed", ErrInvalidToken)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &header); err != nil {
		return nil, fmt.Errorf("%w: header", ErrInvalidToken)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("%w: unexpected alg %q", ErrInvalidToken, header.Alg)
	}
	key, err := c.key(ctx, header.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: signature encoding", ErrInvalidToken)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return nil, fmt.Errorf("%w: bad signature", ErrInvalidToken)
	}

	var claims idClaims
	if err := decodeSegment(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("%w: claims", ErrInvalidToken)
	}
	if !validIssuers[claims.Iss] {
		return nil, fmt.Errorf("%w: issuer", ErrInvalidToken)
	}
	auds := claims.audiences()
	audOK := false
	for _, a := range auds {
		if a == c.ClientID {
			audOK = true
		}
	}
	if !audOK || (len(auds) > 1 && claims.Azp != c.ClientID) || (claims.Azp != "" && claims.Azp != c.ClientID) {
		return nil, fmt.Errorf("%w: audience", ErrInvalidToken)
	}
	now := c.now()
	if claims.Exp == 0 || now.After(time.Unix(claims.Exp, 0).Add(clockSkew)) {
		return nil, fmt.Errorf("%w: expired", ErrInvalidToken)
	}
	if claims.Iat != 0 && time.Unix(claims.Iat, 0).After(now.Add(clockSkew)) {
		return nil, fmt.Errorf("%w: issued in the future", ErrInvalidToken)
	}
	if expectedNonce == "" || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(expectedNonce)) != 1 {
		return nil, fmt.Errorf("%w: nonce", ErrInvalidToken)
	}
	if claims.Sub == "" || claims.Email == "" {
		return nil, fmt.Errorf("%w: missing sub or email", ErrInvalidToken)
	}
	if !claims.emailVerified() {
		return nil, ErrEmailNotVerified
	}
	return &Identity{Subject: claims.Sub, Email: claims.Email}, nil
}

func decodeSegment(seg string, dst any) error {
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

// key returns the signing key for kid, refreshing the JWKS when it has
// expired or does not contain kid (Google rotates keys), at most once a minute.
func (c *Client) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if k, ok := c.keys[kid]; ok && now.Before(c.keysUntil) {
		return k, nil
	}
	if c.keys == nil || now.After(c.keysUntil) || now.Sub(c.lastFetch) >= minRefetchPeriod {
		if err := c.fetchKeys(ctx); err != nil {
			if k, ok := c.keys[kid]; ok {
				return k, nil // stale but known key; better than failing sign-in
			}
			return nil, err
		}
	}
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("%w: unknown signing key", ErrInvalidToken)
}

func (c *Client) fetchKeys(ctx context.Context) error {
	c.lastFetch = c.now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, orDefault(c.JWKSURL, DefaultJWKSURL), nil)
	if err != nil {
		return err
	}
	res, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("fetch Google keys: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch Google keys: status %d", res.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&set); err != nil {
		return fmt.Errorf("fetch Google keys: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") || (k.Alg != "" && k.Alg != "RS256") {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		exp := 0
		for _, b := range e {
			exp = exp<<8 | int(b)
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}
	}
	if len(keys) == 0 {
		return errors.New("fetch Google keys: no usable keys")
	}
	c.keys = keys
	c.keysUntil = c.lastFetch.Add(maxAge(res.Header.Get("Cache-Control")))
	return nil
}

func maxAge(cacheControl string) time.Duration {
	for _, part := range strings.Split(cacheControl, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "max-age="); ok {
			var secs int
			if _, err := fmt.Sscanf(v, "%d", &secs); err == nil && secs > 0 {
				return time.Duration(secs) * time.Second
			}
		}
	}
	return defaultKeysTTL
}
