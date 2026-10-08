package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func adminServer(t *testing.T, roles map[string]string, lookupErr error) (*Auth, http.Handler, *bool) {
	t.Helper()
	auth := NewAuth("test-secret")
	reached := false
	lookup := func(_ context.Context, userID string) (string, error) {
		if lookupErr != nil {
			return "", lookupErr
		}
		role, ok := roles[userID]
		if !ok {
			return "", errors.New("no rows")
		}
		return role, nil
	}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	return auth, auth.Verify(RequireAdmin(lookup)(inner)), &reached
}

func call(t *testing.T, auth *Auth, h http.Handler, userID string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil)
	if userID != "" {
		token, err := auth.GenerateToken(userID)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

func TestRequireAdminAllowsAdmins(t *testing.T) {
	auth, h, reached := adminServer(t, map[string]string{"u-admin": RoleAdmin}, nil)
	if code := call(t, auth, h, "u-admin"); code != http.StatusOK || !*reached {
		t.Fatalf("admin must pass, got %d", code)
	}
}

func TestRequireAdminForbidsOrdinaryUsers(t *testing.T) {
	auth, h, reached := adminServer(t, map[string]string{"u-user": RoleUser}, nil)
	if code := call(t, auth, h, "u-user"); code != http.StatusForbidden || *reached {
		t.Fatalf("ordinary user must get 403, got %d", code)
	}
}

func TestRequireAdminFailsClosed(t *testing.T) {
	auth, h, reached := adminServer(t, map[string]string{}, nil)
	if code := call(t, auth, h, "deleted-user"); code != http.StatusForbidden || *reached {
		t.Fatalf("unknown user must get 403, got %d", code)
	}
	auth, h, reached = adminServer(t, nil, errors.New("db down"))
	if code := call(t, auth, h, "u-admin"); code != http.StatusForbidden || *reached {
		t.Fatalf("lookup failure must not grant access, got %d", code)
	}
}

func TestRequireAdminStillRequiresAuthentication(t *testing.T) {
	auth, h, reached := adminServer(t, map[string]string{"u-admin": RoleAdmin}, nil)
	if code := call(t, auth, h, ""); code != http.StatusUnauthorized || *reached {
		t.Fatalf("anonymous request must get 401, got %d", code)
	}
}

func TestRequireAdminWithoutVerifyIsUnauthorized(t *testing.T) {
	reached := false
	h := RequireAdmin(func(context.Context, string) (string, error) { return RoleAdmin, nil })(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusUnauthorized || reached {
		t.Fatalf("a request with no verified user id must get 401, got %d", w.Code)
	}
}
