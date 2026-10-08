package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func verifyWith(a *Auth, token string) int {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	a.Verify(ok).ServeHTTP(rec, req)
	return rec.Code
}

func TestTokenCarriesSessionVersionAndOlderVersionsAreRejected(t *testing.T) {
	a := NewAuth("test-only-secret")
	current := 2
	a.SetSessionVersionLookup(func(_ context.Context, userID string) (int, error) {
		if userID != "u1" {
			return 0, errors.New("unknown user")
		}
		return current, nil
	})
	old, _ := a.GenerateSessionToken("u1", 1)
	fresh, _ := a.GenerateSessionToken("u1", 2)
	if code := verifyWith(a, old); code != http.StatusUnauthorized {
		t.Fatalf("token from an ended session accepted: %d", code)
	}
	if code := verifyWith(a, fresh); code != http.StatusNoContent {
		t.Fatalf("current session rejected: %d", code)
	}
}

func TestLegacyTokensWithoutVersionCountAsVersionZero(t *testing.T) {
	a := NewAuth("test-only-secret")
	version := 0
	a.SetSessionVersionLookup(func(context.Context, string) (int, error) { return version, nil })
	legacy, _ := a.GenerateToken("u1")
	if code := verifyWith(a, legacy); code != http.StatusNoContent {
		t.Fatalf("legacy token rejected before any password change: %d", code)
	}
	version = 1
	if code := verifyWith(a, legacy); code != http.StatusUnauthorized {
		t.Fatalf("legacy token accepted after a password change: %d", code)
	}
}

func TestDeletedUsersTokensAreRejected(t *testing.T) {
	a := NewAuth("test-only-secret")
	a.SetSessionVersionLookup(func(context.Context, string) (int, error) { return 0, ErrSessionUserGone })
	token, _ := a.GenerateToken("gone")
	if code := verifyWith(a, token); code != http.StatusUnauthorized {
		t.Fatalf("token of a deleted account accepted: %d", code)
	}
}

func TestLookupOutageIsNotTreatedAsSignedOut(t *testing.T) {
	a := NewAuth("test-only-secret")
	a.SetSessionVersionLookup(func(context.Context, string) (int, error) { return 0, errors.New("db down") })
	token, _ := a.GenerateToken("u1")
	if code := verifyWith(a, token); code != http.StatusServiceUnavailable {
		t.Fatalf("database outage should be 503 so the console keeps the cookie, got %d", code)
	}
}
