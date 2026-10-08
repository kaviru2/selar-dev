package middleware

import (
	"context"
	"net/http"
)

// User roles stored in users.role.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// RoleLookup returns the current role of a user from the database.
type RoleLookup func(ctx context.Context, userID string) (string, error)

// RequireAdmin must run after Auth.Verify. The role is read from the
// database on every request (it is deliberately not a JWT claim), so a
// demotion takes effect immediately. Any lookup failure denies access.
func RequireAdmin(lookup RoleLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r.Context())
			if userID == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			role, err := lookup(r.Context(), userID)
			if err != nil || role != RoleAdmin {
				writeError(w, http.StatusForbidden, "admin access required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
