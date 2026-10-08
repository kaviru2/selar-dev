package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/store"
)

func sendJSON(t *testing.T, fn http.HandlerFunc, method, path, body, userID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req = authed(req, userID)
	}
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

// Validation must reject bad input before touching the store (nil here).
func TestSettingsValidationHappensBeforePersistence(t *testing.T) {
	h := New(nil)
	const user = "00000000-0000-0000-0000-000000000001"
	cases := []struct {
		name string
		fn   http.HandlerFunc
		body string
	}{
		{"unknown setting", h.UpdateSettings, `{"density":"compact"}`},
		{"bad colour scheme", h.UpdateSettings, `{"colorScheme":"paper"}`},
		{"empty patch", h.UpdateSettings, `{}`},
		{"not json", h.UpdateSettings, `nope`},
		{"long name", h.UpdateProfile, `{"display_name":"` + strings.Repeat("x", 81) + `"}`},
		{"bad email", h.ChangeEmail, `{"email":"not-an-email","current_password":"whatever"}`},
		{"missing password", h.ChangeEmail, `{"email":"selar.qa+a@example.com"}`},
		{"missing current", h.ChangePassword, `{"new_password":"correct horse 42"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := sendJSON(t, c.fn, http.MethodPost, "/", c.body, user); rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

type accountFixture struct {
	h     *Handler
	auth  *middleware.Auth
	pool  *pgxpool.Pool
	st    *store.Store
	id    string
	email string
}

const fixturePassword = "synthetic pass 1"

func newAccountFixture(t *testing.T) *accountFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(fixturePassword), bcrypt.MinCost)
	f := &accountFixture{pool: pool, st: store.New(pool)}
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,cohort)
		VALUES ('selar.qa+'||gen_random_uuid()::text||'@example.com',$1,'control') RETURNING id, email`, string(hash)).Scan(&f.id, &f.email); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, f.id)
		_, _ = pool.Exec(ctx, `DELETE FROM cohort_setting_locks WHERE reason = 'handler-test'`)
		pool.Close()
	})
	f.h = New(f.st)
	f.auth = middleware.NewAuth("test-only-secret")
	f.auth.SetSessionVersionLookup(SessionVersionLookup(f.st))
	f.h.SetAuth(f.auth)
	return f
}

func (f *accountFixture) router() http.Handler {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(f.auth.Verify)
		r.Get("/api/users/me", f.h.GetCurrentUser)
		r.Post("/api/users/me/password", f.h.ChangePassword)
	})
	return r
}

func TestIntegrationSettingsMergeDefaultsAndCohortLock(t *testing.T) {
	f := newAccountFixture(t)
	if rec := sendJSON(t, f.h.UpdateSettings, http.MethodPatch, "/", `{"colorScheme":"dark"}`, f.id); rec.Code != http.StatusOK {
		t.Fatalf("colour scheme: %d %s", rec.Code, rec.Body.String())
	}
	if rec := sendJSON(t, f.h.UpdateSettings, http.MethodPatch, "/", `{"reader.zoom":1.4}`, f.id); rec.Code != http.StatusOK {
		t.Fatalf("zoom: %d %s", rec.Code, rec.Body.String())
	}
	rec := sendJSON(t, f.h.GetSettings, http.MethodGet, "/", "", f.id)
	var got struct {
		Values map[string]any `json:"values"`
		Locked []string       `json:"locked"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Values["colorScheme"] != "dark" || got.Values["reader.zoom"] != 1.4 || got.Values["reader.highlight_color"] != "yellow" {
		t.Fatalf("settings = %#v", got.Values)
	}

	if err := f.st.SetCohortSettingLock(context.Background(), "control", "suggestions.show_on_open", true, "handler-test"); err != nil {
		t.Fatal(err)
	}
	rec = sendJSON(t, f.h.UpdateSettings, http.MethodPatch, "/", `{"suggestions.show_on_open":false}`, f.id)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "locked") {
		t.Fatalf("locked setting accepted: %d %s", rec.Code, rec.Body.String())
	}
	rec = sendJSON(t, f.h.GetSettings, http.MethodGet, "/", "", f.id)
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Values["suggestions.show_on_open"] != true || len(got.Locked) != 1 {
		t.Fatalf("lock not reported: %#v %#v", got.Values, got.Locked)
	}
}

func TestIntegrationLegacyPreferencesRouteMergesAndIgnoresUnknownKeys(t *testing.T) {
	f := newAccountFixture(t)
	sendJSON(t, f.h.UpdateSettings, http.MethodPatch, "/", `{"colorScheme":"dark"}`, f.id)
	// Older consoles send their whole preference object, including retired
	// keys such as density. That must neither fail nor erase other settings.
	rec := sendJSON(t, f.h.UpdatePreferences, http.MethodPatch, "/", `{"theme":"warm","density":"compact"}`, f.id)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy route: %d %s", rec.Code, rec.Body.String())
	}
	stored, _, _, _ := f.st.GetUserSettings(context.Background(), f.id)
	if stored["colorScheme"] != "dark" || stored["theme"] != "warm" {
		t.Fatalf("legacy preferences route lost a setting: %#v", stored)
	}
	if _, ok := stored["density"]; ok {
		t.Fatal("unknown legacy key was stored")
	}
}

func TestIntegrationProfileAndEmailChange(t *testing.T) {
	f := newAccountFixture(t)
	if rec := sendJSON(t, f.h.UpdateProfile, http.MethodPatch, "/", `{"display_name":"  Synthetic   Tester "}`, f.id); rec.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", rec.Code, rec.Body.String())
	}
	u, _ := f.st.GetUserByID(context.Background(), f.id)
	if u.DisplayName != "Synthetic Tester" {
		t.Fatalf("display name %q", u.DisplayName)
	}

	newEmail := "selar.qa+changed-" + f.id + "@example.com"
	if rec := sendJSON(t, f.h.ChangeEmail, http.MethodPost, "/", `{"email":"`+newEmail+`","current_password":"wrong password 9"}`, f.id); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong password accepted for email change: %d", rec.Code)
	}
	if rec := sendJSON(t, f.h.ChangeEmail, http.MethodPost, "/", `{"email":"`+newEmail+`","current_password":"`+fixturePassword+`"}`, f.id); rec.Code != http.StatusOK {
		t.Fatalf("email change: %d %s", rec.Code, rec.Body.String())
	}
	u, _ = f.st.GetUserByID(context.Background(), f.id)
	if u.Email != newEmail {
		t.Fatalf("email %q", u.Email)
	}
}

func TestIntegrationPasswordChangeEndsOtherSessions(t *testing.T) {
	f := newAccountFixture(t)
	router := f.router()
	oldToken, _ := f.auth.GenerateToken(f.id)
	call := func(token, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(oldToken, http.MethodGet, "/api/users/me", ""); rec.Code != http.StatusOK {
		t.Fatalf("baseline session rejected: %d", rec.Code)
	}
	if rec := call(oldToken, http.MethodPost, "/api/users/me/password", `{"current_password":"wrong password 9","new_password":"brand new pass 7"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong current password: %d", rec.Code)
	}
	if rec := call(oldToken, http.MethodPost, "/api/users/me/password", `{"current_password":"`+fixturePassword+`","new_password":"aaaaaaaaaaaa"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password: %d", rec.Code)
	}
	rec := call(oldToken, http.MethodPost, "/api/users/me/password", `{"current_password":"`+fixturePassword+`","new_password":"brand new pass 7"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("password change: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Token == "" {
		t.Fatal("no replacement token for the current session")
	}
	if rec := call(oldToken, http.MethodGet, "/api/users/me", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old session still valid after password change: %d", rec.Code)
	}
	if rec := call(body.Token, http.MethodGet, "/api/users/me", ""); rec.Code != http.StatusOK {
		t.Fatalf("replacement token rejected: %d", rec.Code)
	}
	_, hash, _, _ := f.st.GetUserAuth(context.Background(), f.id)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("brand new pass 7")) != nil {
		t.Fatal("new password not stored")
	}
	// Login issues a token at the current session version.
	login := sendJSON(t, f.h.Login, http.MethodPost, "/auth/login", `{"email":"`+f.email+`","password":"brand new pass 7"}`, "")
	_ = json.Unmarshal(login.Body.Bytes(), &body)
	if rec := call(body.Token, http.MethodGet, "/api/users/me", ""); rec.Code != http.StatusOK {
		t.Fatalf("fresh login token rejected after password change: %d", rec.Code)
	}
}
