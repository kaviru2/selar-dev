package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/selar-dev/selar-api/internal/googleauth"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/store"
)

func TestResolveGoogleAccountRules(t *testing.T) {
	id := &googleauth.Identity{Subject: "sub-1", Email: "Learner@Example.com"}
	user := &model.User{ID: "u1", Email: "learner@example.com"}
	cases := []struct {
		name    string
		bySub   *model.User
		byEmail *store.GoogleAccount
		want    googleAction
	}{
		{"already linked by subject", user, nil, googleSignIn},
		{"subject wins over a different email account", user, &store.GoogleAccount{User: &model.User{ID: "u2", Email: "learner@example.com"}}, googleSignIn},
		{"verified email equals existing account email", nil, &store.GoogleAccount{User: user}, googleLink},
		{"existing account already linked to this subject", nil, &store.GoogleAccount{User: user, GoogleSub: "sub-1"}, googleLink},
		{"existing account linked to another Google account", nil, &store.GoogleAccount{User: user, GoogleSub: "sub-2"}, googleConflict},
		{"no account", nil, nil, googleCreate},
		{"store returned a different email", nil, &store.GoogleAccount{User: &model.User{ID: "u3", Email: "other@example.com"}}, googleCreate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveGoogleAccount(id, c.bySub, c.byEmail); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

type fakeGoogle struct {
	id  *googleauth.Identity
	err error
	got []string
}

func (f *fakeGoogle) Exchange(_ context.Context, code, verifier, redirectURI, nonce string) (*googleauth.Identity, error) {
	f.got = []string{code, verifier, redirectURI, nonce}
	return f.id, f.err
}

const verifier = "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv" // 50 chars

func googleBody(code string) string {
	b, _ := json.Marshal(map[string]string{"code": code, "code_verifier": verifier, "nonce": "n", "redirect_uri": "https://console.test/api/auth/google/callback"})
	return string(b)
}

func TestGoogleSignInDisabledAndValidation(t *testing.T) {
	h := New(nil)
	if rec := sendJSON(t, h.GoogleSignIn, http.MethodPost, "/auth/google", googleBody("c"), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("disabled: status %d", rec.Code)
	}
	f := &fakeGoogle{}
	h.SetGoogle(f)
	h.SetAuth(nil)
	if rec := sendJSON(t, h.GoogleSignIn, http.MethodPost, "/auth/google", googleBody("c"), ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("no auth: status %d", rec.Code)
	}
	fx := New(nil)
	fx.SetGoogle(f)
	fx.SetAuth(newTestAuth())
	for name, body := range map[string]string{
		"missing code":    `{"code_verifier":"` + verifier + `","nonce":"n","redirect_uri":"r"}`,
		"short verifier":  `{"code":"c","code_verifier":"short","nonce":"n","redirect_uri":"r"}`,
		"unknown field":   `{"code":"c","code_verifier":"` + verifier + `","nonce":"n","redirect_uri":"r","email":"x"}`,
		"missing nonce":   `{"code":"c","code_verifier":"` + verifier + `","redirect_uri":"r"}`,
		"not json at all": `nope`,
	} {
		if rec := sendJSON(t, fx.GoogleSignIn, http.MethodPost, "/auth/google", body, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
	if f.got != nil {
		t.Fatal("exchange must not run for invalid requests")
	}
}

func TestGoogleSignInRefusesUnverifiedAndInvalidTokens(t *testing.T) {
	h := New(nil)
	h.SetAuth(newTestAuth())
	h.SetGoogle(&fakeGoogle{err: googleauth.ErrEmailNotVerified})
	if rec := sendJSON(t, h.GoogleSignIn, http.MethodPost, "/auth/google", googleBody("c"), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("unverified: status %d", rec.Code)
	}
	h.SetGoogle(&fakeGoogle{err: errors.Join(googleauth.ErrInvalidToken, errors.New("aud"))})
	if rec := sendJSON(t, h.GoogleSignIn, http.MethodPost, "/auth/google", googleBody("c"), ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid: status %d", rec.Code)
	}
}

// The rest needs Postgres (TEST_DATABASE_URL), like the other account tests.

func googleResult(t *testing.T, rec *httptest.ResponseRecorder) (string, *model.User, bool) {
	t.Helper()
	var out struct {
		Token   string      `json:"token"`
		User    *model.User `json:"user"`
		Created bool        `json:"created"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %s", rec.Body.String())
	}
	return out.Token, out.User, out.Created
}

func TestIntegrationGoogleSignInCreatesThenSignsIn(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	email := "selar.qa+google-" + f.id[:8] + "@example.com"
	sub := "google-sub-" + f.id
	t.Cleanup(func() { _, _ = f.pool.Exec(ctx, `DELETE FROM users WHERE google_sub = $1`, sub) })
	f.h.SetGoogle(&fakeGoogle{id: &googleauth.Identity{Subject: sub, Email: email}})

	rec := sendJSON(t, f.h.GoogleSignIn, http.MethodPost, "/auth/google", googleBody("c"), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status %d %s", rec.Code, rec.Body.String())
	}
	token, user, created := googleResult(t, rec)
	if !created || user.Email != email || token == "" {
		t.Fatalf("created=%v user=%+v", created, user)
	}
	// Same registration path: same cohort as Register, consent left unasked.
	reg := New(nil).newRegistration(email, "x", nil, sub)
	if user.Cohort != reg.Cohort || user.Role != "user" || user.ConsentDecidedAt != nil || user.ConsentedAt != nil {
		t.Fatalf("new Google user %+v differs from registration defaults %+v", user, reg)
	}
	// The issued token is a normal session token.
	req := httptest.NewRequest(http.MethodGet, "/api/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	me := httptest.NewRecorder()
	f.router().ServeHTTP(me, req)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), email) {
		t.Fatalf("/users/me with Google token: %d %s", me.Code, me.Body.String())
	}
	// Nobody knows the generated password: password login must fail.
	login := sendJSON(t, f.h.Login, http.MethodPost, "/auth/login", `{"email":"`+email+`","password":""}`, "")
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("password login on a Google-only account: %d", login.Code)
	}

	// Second sign-in finds the account by subject, even if the Google email changed.
	f.h.SetGoogle(&fakeGoogle{id: &googleauth.Identity{Subject: sub, Email: "renamed-" + email}})
	rec = sendJSON(t, f.h.GoogleSignIn, http.MethodPost, "/auth/google", googleBody("c"), "")
	_, again, created := googleResult(t, rec)
	if rec.Code != http.StatusOK || created || again.ID != user.ID {
		t.Fatalf("second sign-in: %d created=%v id=%s want %s", rec.Code, created, again.ID, user.ID)
	}
}

func TestIntegrationGoogleSignInLinksVerifiedMatchingEmail(t *testing.T) {
	f := newAccountFixture(t)
	sub := "google-sub-link-" + f.id
	// Google reports the address in a different case; it is the same mailbox.
	f.h.SetGoogle(&fakeGoogle{id: &googleauth.Identity{Subject: sub, Email: strings.ToUpper(f.email)}})

	rec := sendJSON(t, f.h.GoogleSignIn, http.MethodPost, "/auth/google", googleBody("c"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("link: status %d %s", rec.Code, rec.Body.String())
	}
	_, user, created := googleResult(t, rec)
	if created || user.ID != f.id {
		t.Fatalf("linked to %s (created=%v), want existing %s", user.ID, created, f.id)
	}
	var stored string
	if err := f.pool.QueryRow(context.Background(), `SELECT google_sub FROM users WHERE id = $1`, f.id).Scan(&stored); err != nil || stored != sub {
		t.Fatalf("google_sub = %q, %v", stored, err)
	}
	// The password keeps working after linking.
	login := sendJSON(t, f.h.Login, http.MethodPost, "/auth/login", `{"email":"`+f.email+`","password":"`+fixturePassword+`"}`, "")
	if login.Code != http.StatusOK {
		t.Fatalf("password login after link: %d", login.Code)
	}

	// A different Google account with the same email is refused, not re-linked.
	f.h.SetGoogle(&fakeGoogle{id: &googleauth.Identity{Subject: "someone-else-" + f.id, Email: f.email}})
	rec = sendJSON(t, f.h.GoogleSignIn, http.MethodPost, "/auth/google", googleBody("c"), "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("second Google account: status %d %s", rec.Code, rec.Body.String())
	}
}

func newTestAuth() *middleware.Auth { return middleware.NewAuth("test-only-secret") }
