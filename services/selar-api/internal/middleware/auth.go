// Package middleware provides HTTP middleware for the SELAR API.
// auth.go implements a lightweight HMAC-SHA256 JWT authentication system
// without external JWT libraries to keep the dependency count minimal.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type contextKey string

const UserIDKey contextKey = "user_id"

// ErrSessionUserGone tells Verify that the token's user no longer exists.
var ErrSessionUserGone = errors.New("user no longer exists")

// SessionVersionLookup returns the user's current session version.
type SessionVersionLookup func(ctx context.Context, userID string) (int, error)

// Auth provides JWT-based authentication middleware.
type Auth struct {
	secret        []byte
	sessionLookup SessionVersionLookup
}

// SetSessionVersionLookup enables server-side session revocation: a token
// whose "sv" claim is older than the user's current session version (or whose
// user was deleted) is rejected. Without a lookup only signature and expiry
// are checked.
func (a *Auth) SetSessionVersionLookup(lookup SessionVersionLookup) {
	a.sessionLookup = lookup
}

// NewAuth creates a new Auth middleware with the given JWT secret.
func NewAuth(secret string) *Auth {
	return &Auth{secret: []byte(secret)}
}

// Verify is the middleware that checks for a valid JWT in the Authorization header.
func (a *Auth) Verify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		claims, err := a.validateToken(parts[1])
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
			return
		}

		if a.sessionLookup != nil {
			current, err := a.sessionLookup(r.Context(), claims.Sub)
			if errors.Is(err, ErrSessionUserGone) {
				http.Error(w, `{"error":"account no longer exists"}`, http.StatusUnauthorized)
				return
			}
			if err != nil {
				http.Error(w, `{"error":"session check unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			if claims.SV < current {
				http.Error(w, `{"error":"session ended"}`, http.StatusUnauthorized)
				return
			}
		}

		ctx := context.WithValue(r.Context(), UserIDKey, claims.Sub)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GenerateToken creates a signed JWT for the given user ID at session
// version 0 (an account whose password has never been changed).
func (a *Auth) GenerateToken(userID string) (string, error) {
	return a.GenerateSessionToken(userID, 0)
}

// GenerateSessionToken creates a signed JWT bound to a session version.
func (a *Auth) GenerateSessionToken(userID string, sessionVersion int) (string, error) {
	header := base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := jwtClaims{
		SV:  sessionVersion,
		Sub: userID,
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64URLEncode(claimsJSON)
	sigInput := header + "." + payload
	sig := a.sign([]byte(sigInput))
	return sigInput + "." + base64URLEncode(sig), nil
}

// GetUserID extracts the user ID from the request context.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok {
		return v
	}
	return ""
}

// --- internal JWT helpers ---

type jwtClaims struct {
	Sub string `json:"sub"`
	// SV is the session version; absent in tokens issued before revocation
	// existed, which therefore decode as 0.
	SV  int   `json:"sv,omitempty"`
	Iat int64 `json:"iat"`
	Exp int64 `json:"exp"`
}

func (a *Auth) validateToken(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed token")
	}

	sigInput := parts[0] + "." + parts[1]
	expectedSig := a.sign([]byte(sigInput))
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, fmt.Errorf("invalid signature")
	}

	claimsJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid claims encoding")
	}

	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims")
	}

	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func (a *Auth) sign(data []byte) []byte {
	h := hmac.New(sha256.New, a.secret)
	h.Write(data)
	return h.Sum(nil)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
