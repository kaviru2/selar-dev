package googleauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testClientID = "test-client.apps.googleusercontent.com"

var (
	testNow = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	keyA    = mustKey()
	keyB    = mustKey()
)

func mustKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// mockJWKS serves the public halves of the given keys and counts fetches.
func mockJWKS(t *testing.T, keys map[string]*rsa.PrivateKey) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		var set struct {
			Keys []map[string]string `json:"keys"`
		}
		for kid, k := range keys {
			set.Keys = append(set.Keys, map[string]string{
				"kty": "RSA", "alg": "RS256", "use": "sig", "kid": kid,
				"n": b64(k.N.Bytes()), "e": b64(big.NewInt(int64(k.E)).Bytes()),
			})
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(set)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func sign(t *testing.T, key *rsa.PrivateKey, kid, alg string, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
	c, _ := json.Marshal(claims)
	input := b64(h) + "." + b64(c)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + b64(sig)
}

func goodClaims() map[string]any {
	return map[string]any{
		"iss": "https://accounts.google.com", "aud": testClientID, "azp": testClientID,
		"sub": "1234567890", "email": "learner@example.com", "email_verified": true,
		"nonce": "n-123", "iat": testNow.Add(-time.Minute).Unix(), "exp": testNow.Add(time.Hour).Unix(),
		// Present in real tokens with the profile scope; must be ignored.
		"name": "Should Not Be Read", "picture": "https://example.com/p.png",
	}
}

func newClient(jwksURL string) *Client {
	return &Client{ClientID: testClientID, ClientSecret: "test-secret", JWKSURL: jwksURL, Now: func() time.Time { return testNow }}
}

func TestVerifyAcceptsValidToken(t *testing.T) {
	srv, _ := mockJWKS(t, map[string]*rsa.PrivateKey{"a": keyA})
	id, err := newClient(srv.URL).Verify(context.Background(), sign(t, keyA, "a", "RS256", goodClaims()), "n-123")
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "1234567890" || id.Email != "learner@example.com" {
		t.Fatalf("identity %+v", id)
	}
}

func TestVerifyRejects(t *testing.T) {
	srv, _ := mockJWKS(t, map[string]*rsa.PrivateKey{"a": keyA})
	cases := []struct {
		name   string
		key    *rsa.PrivateKey
		kid    string
		alg    string
		mutate func(map[string]any)
		nonce  string
		want   error
	}{
		{name: "signature by an unknown key", key: keyB, kid: "a", want: ErrInvalidToken},
		{name: "unknown kid", key: keyA, kid: "zzz", want: ErrInvalidToken},
		{name: "alg none/HS256", key: keyA, kid: "a", alg: "HS256", want: ErrInvalidToken},
		{name: "wrong audience", mutate: func(c map[string]any) { c["aud"] = "someone-else"; c["azp"] = "someone-else" }, want: ErrInvalidToken},
		{name: "audience list without our azp", mutate: func(c map[string]any) { c["aud"] = []string{testClientID, "x"}; c["azp"] = "x" }, want: ErrInvalidToken},
		{name: "wrong issuer", mutate: func(c map[string]any) { c["iss"] = "https://evil.example.com" }, want: ErrInvalidToken},
		{name: "expired", mutate: func(c map[string]any) { c["exp"] = testNow.Add(-10 * time.Minute).Unix() }, want: ErrInvalidToken},
		{name: "missing exp", mutate: func(c map[string]any) { delete(c, "exp") }, want: ErrInvalidToken},
		{name: "issued in the future", mutate: func(c map[string]any) { c["iat"] = testNow.Add(time.Hour).Unix() }, want: ErrInvalidToken},
		{name: "nonce mismatch", nonce: "other", want: ErrInvalidToken},
		{name: "empty expected nonce", nonce: "-", mutate: func(c map[string]any) { c["nonce"] = "" }, want: ErrInvalidToken},
		{name: "missing sub", mutate: func(c map[string]any) { delete(c, "sub") }, want: ErrInvalidToken},
		{name: "email not verified", mutate: func(c map[string]any) { c["email_verified"] = false }, want: ErrEmailNotVerified},
		{name: "email_verified absent", mutate: func(c map[string]any) { delete(c, "email_verified") }, want: ErrEmailNotVerified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := goodClaims()
			if tc.mutate != nil {
				tc.mutate(claims)
			}
			key, kid, alg, nonce := keyA, "a", "RS256", "n-123"
			if tc.key != nil {
				key, kid = tc.key, tc.kid
			}
			if tc.alg != "" {
				alg = tc.alg
			}
			if tc.nonce == "-" {
				nonce = ""
			} else if tc.nonce != "" {
				nonce = tc.nonce
			}
			_, err := newClient(srv.URL).Verify(context.Background(), sign(t, key, kid, alg, claims), nonce)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	srv, _ := mockJWKS(t, map[string]*rsa.PrivateKey{"a": keyA})
	tok := sign(t, keyA, "a", "RS256", goodClaims())
	parts := strings.Split(tok, ".")
	c := goodClaims()
	c["email"] = "victim@example.com"
	raw, _ := json.Marshal(c)
	parts[1] = b64(raw)
	if _, err := newClient(srv.URL).Verify(context.Background(), strings.Join(parts, "."), "n-123"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyAcceptsLegacyIssuerAndStringEmailVerified(t *testing.T) {
	srv, _ := mockJWKS(t, map[string]*rsa.PrivateKey{"a": keyA})
	c := goodClaims()
	c["iss"] = "accounts.google.com"
	c["email_verified"] = "true"
	if _, err := newClient(srv.URL).Verify(context.Background(), sign(t, keyA, "a", "RS256", c), "n-123"); err != nil {
		t.Fatal(err)
	}
}

func TestKeysAreCachedAndRefetchedOnRotation(t *testing.T) {
	keys := map[string]*rsa.PrivateKey{"a": keyA}
	srv, hits := mockJWKS(t, keys)
	now := testNow
	cl := newClient(srv.URL)
	cl.Now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if _, err := cl.Verify(context.Background(), sign(t, keyA, "a", "RS256", goodClaims()), "n-123"); err != nil {
			t.Fatal(err)
		}
	}
	if *hits != 1 {
		t.Fatalf("JWKS fetched %d times, want 1 (cached)", *hits)
	}
	// Google rotates in key "b". An unknown kid triggers one refetch.
	keys["b"] = keyB
	now = now.Add(2 * time.Minute)
	if _, err := cl.Verify(context.Background(), sign(t, keyB, "b", "RS256", goodClaims()), "n-123"); err != nil {
		t.Fatal(err)
	}
	if *hits != 2 {
		t.Fatalf("JWKS fetched %d times, want 2", *hits)
	}
}

func TestExchangeSendsPKCEAndVerifies(t *testing.T) {
	jwks, _ := mockJWKS(t, map[string]*rsa.PrivateKey{"a": keyA})
	var got url.Values
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostForm
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": sign(t, keyA, "a", "RS256", goodClaims()), "access_token": "ignored"})
	}))
	defer token.Close()
	cl := newClient(jwks.URL)
	cl.TokenURL = token.URL
	id, err := cl.Exchange(context.Background(), "the-code", "the-verifier", "https://console.test/api/auth/google/callback", "n-123")
	if err != nil {
		t.Fatal(err)
	}
	if id.Email != "learner@example.com" {
		t.Fatalf("identity %+v", id)
	}
	want := map[string]string{"grant_type": "authorization_code", "code": "the-code", "code_verifier": "the-verifier",
		"redirect_uri": "https://console.test/api/auth/google/callback", "client_id": testClientID, "client_secret": "test-secret"}
	for k, v := range want {
		if got.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, got.Get(k), v)
		}
	}
}

func TestExchangeSurfacesGoogleErrorWithoutSecrets(t *testing.T) {
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Bad Request"}`))
	}))
	defer token.Close()
	cl := newClient("http://unused")
	cl.TokenURL = token.URL
	_, err := cl.Exchange(context.Background(), "c", "v", "r", "n")
	if err == nil || !strings.Contains(err.Error(), "invalid_grant") || strings.Contains(err.Error(), "test-secret") {
		t.Fatalf("err = %v", err)
	}
}

func TestFromEnvRequiresBothValues(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if FromEnv(env(map[string]string{"GOOGLE_CLIENT_ID": "x"})) != nil {
		t.Fatal("enabled without a secret")
	}
	if FromEnv(env(map[string]string{"GOOGLE_CLIENT_SECRET": "x"})) != nil {
		t.Fatal("enabled without a client id")
	}
	if FromEnv(env(map[string]string{"GOOGLE_CLIENT_ID": "x", "GOOGLE_CLIENT_SECRET": "y"})) == nil {
		t.Fatal("not enabled with both values")
	}
	var nilClient *Client
	if _, err := nilClient.Exchange(context.Background(), "c", "v", "r", "n"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}
