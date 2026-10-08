package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/analytics"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/store"
)

func TestTrackIsANoOpWithoutStoreOrWhenDisabled(t *testing.T) {
	h := New(nil)
	// Must not panic without a database and must not block the request.
	h.Track(context.Background(), testUser, analytics.SignedIn, analytics.Props{"method": "login"})
	h.SetAnalyticsEnabled(false)
	h.Track(context.Background(), testUser, analytics.SignedIn, nil)
}

func TestCollectRejectsOversizedBatches(t *testing.T) {
	h := New(nil)
	events := make([]map[string]any, analytics.MaxBatch+1)
	for i := range events {
		events[i] = map[string]any{"event": "page_viewed", "props": map[string]any{"route": "/library"}}
	}
	w := postJSON(t, h.CollectEvents, "/api/analytics/events", map[string]any{"events": events}, testUser)
	if w.Code != http.StatusRequestEntityTooLarge && w.Code != http.StatusBadRequest {
		t.Fatalf("oversized batch: got %d", w.Code)
	}
}

func TestCollectReturnsAcceptedWhenAnalyticsDisabled(t *testing.T) {
	h := New(nil)
	h.SetAnalyticsEnabled(false)
	w := postJSON(t, h.CollectEvents, "/api/analytics/events", map[string]any{"events": []map[string]any{{"event": "page_viewed"}}}, testUser)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("disabled analytics must be a silent 202, got %d %s", w.Code, w.Body.String())
	}
}

func TestAdminFilterParsing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/overview?from=2026-10-01&to=2026-10-07&group=A", nil)
	f, err := parseAdminFilter(r)
	if err != nil {
		t.Fatal(err)
	}
	if f.From.Format("2006-01-02") != "2026-10-01" || f.To.Format("2006-01-02") != "2026-10-08" || f.Group != "A" {
		t.Fatalf("unexpected filter: %+v (to is exclusive, so the end day is included)", f)
	}
	for _, bad := range []string{"?from=nope", "?from=2026-10-07&to=2026-10-01", "?from=2020-01-01&to=2026-10-01"} {
		if _, err := parseAdminFilter(httptest.NewRequest(http.MethodGet, "/x"+bad, nil)); err == nil {
			t.Fatalf("filter %s must be rejected", bad)
		}
	}
}

// --- Postgres-backed integration -------------------------------------------

type adminFixture struct {
	pool   *pgxpool.Pool
	router http.Handler
	auth   *middleware.Auth
	h      *Handler
}

func newAdminFixture(t *testing.T, adminEmails string) *adminFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	auth := middleware.NewAuth("integration-secret")
	h := New(store.New(pool))
	h.SetAuth(auth)
	h.SetAdminEmails(analytics.ParseEmailList(adminEmails))
	r := chi.NewRouter()
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Route("/api", func(r chi.Router) {
		r.Use(auth.Verify)
		h.MountUserAnalytics(r)
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.RequireAdmin(h.RoleLookup()))
			h.MountAdmin(r)
		})
	})
	return &adminFixture{pool: pool, router: r, auth: auth, h: h}
}

func (f *adminFixture) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

type authResp struct {
	Token string `json:"token"`
	User  struct {
		ID          string  `json:"id"`
		Role        string  `json:"role"`
		ConsentedAt *string `json:"consented_at"`
	} `json:"user"`
}

func (f *adminFixture) register(t *testing.T, email string, consent *bool) authResp {
	t.Helper()
	body := map[string]any{"email": email, "password": "correct-horse-1"}
	if consent != nil {
		body["research_consent"] = *consent
	}
	w := f.do(t, http.MethodPost, "/auth/register", "", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", email, w.Code, w.Body.String())
	}
	var resp authResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, resp.User.ID) })
	return resp
}

func TestIntegrationAdminEmailsBootstrapAndForbidOthers(t *testing.T) {
	adminEmail := "selar.qa+admin-" + uuid.NewString()[:8] + "@example.com"
	f := newAdminFixture(t, strings.ToUpper(adminEmail))
	yes := true
	admin := f.register(t, adminEmail, &yes)
	user := f.register(t, "selar.qa+user-"+uuid.NewString()[:8]+"@example.com", nil)
	if admin.User.Role != "admin" || user.User.Role != "user" {
		t.Fatalf("roles after registration: admin=%q user=%q", admin.User.Role, user.User.Role)
	}
	if admin.User.ConsentedAt == nil {
		t.Fatal("research_consent=true at registration must record consent")
	}
	if w := f.do(t, http.MethodGet, "/api/admin/overview", user.Token, nil); w.Code != http.StatusForbidden {
		t.Fatalf("ordinary user must get 403 from admin routes, got %d", w.Code)
	}
	if w := f.do(t, http.MethodGet, "/api/admin/overview", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous must get 401, got %d", w.Code)
	}
	w := f.do(t, http.MethodGet, "/api/admin/overview", admin.Token, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"decisions"`) {
		t.Fatalf("admin overview: %d %s", w.Code, w.Body.String())
	}

	// An existing account listed in ADMIN_EMAILS later is promoted on login.
	_, _ = f.pool.Exec(context.Background(), `UPDATE users SET role='user' WHERE id=$1`, admin.User.ID)
	w = f.do(t, http.MethodPost, "/auth/login", "", map[string]string{"email": adminEmail, "password": "correct-horse-1"})
	var login authResp
	_ = json.Unmarshal(w.Body.Bytes(), &login)
	if w.Code != http.StatusOK || login.User.Role != "admin" {
		t.Fatalf("login must re-apply ADMIN_EMAILS: %d role=%q", w.Code, login.User.Role)
	}
}

func TestIntegrationConsentBeaconExportAndDeleteMyData(t *testing.T) {
	adminEmail := "selar.qa+admin-" + uuid.NewString()[:8] + "@example.com"
	f := newAdminFixture(t, adminEmail)
	admin := f.register(t, adminEmail, nil)
	no := false
	participant := f.register(t, "selar.qa+p-"+uuid.NewString()[:8]+"@example.com", &no)

	beacon := map[string]any{"events": []map[string]any{
		{"event": "page_viewed", "props": map[string]any{"route": "/reader?doc=x"}},
		{"event": "explain_submitted", "props": map[string]any{"length": 120, "text": "my private explanation"}},
		{"event": "decision_made", "props": map[string]any{"action": "keep"}}, // server-only: refused
	}}
	if w := f.do(t, http.MethodPost, "/api/analytics/events", participant.Token, beacon); w.Code != http.StatusAccepted {
		t.Fatalf("beacon: %d %s", w.Code, w.Body.String())
	}
	var n int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM analytics_events WHERE user_id=$1`, participant.User.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("declined consent: no per-user events allowed, got %d", n)
	}

	if w := f.do(t, http.MethodPut, "/api/users/me/research-consent", participant.Token, map[string]any{"granted": true}); w.Code != http.StatusOK {
		t.Fatalf("grant consent: %d %s", w.Code, w.Body.String())
	}
	_ = f.do(t, http.MethodPost, "/api/analytics/events", participant.Token, beacon)
	rows, _ := f.pool.Query(context.Background(), `SELECT event, props::text FROM analytics_events WHERE user_id=$1 ORDER BY id`, participant.User.ID)
	var got []string
	for rows.Next() {
		var e, p string
		_ = rows.Scan(&e, &p)
		got = append(got, e+" "+p)
	}
	rows.Close()
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "private explanation") || strings.Contains(joined, "doc=x") || strings.Contains(joined, "\ndecision_made") {
		t.Fatalf("free text, query strings or forged server events leaked:\n%s", joined)
	}
	if !strings.Contains(joined, "explain_submitted") || !strings.Contains(joined, `"route": "/reader"`) {
		t.Fatalf("expected sanitized events (plus the consent event), got:\n%s", joined)
	}

	// Admin labels the participant and exports without emails by default.
	path := "/api/admin/users/" + participant.User.ID
	if w := f.do(t, http.MethodPatch, path, admin.Token, map[string]any{"group_label": "Group A"}); w.Code != http.StatusOK {
		t.Fatalf("set group label: %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, http.MethodPatch, path, participant.Token, map[string]any{"group_label": "B"}); w.Code != http.StatusForbidden {
		t.Fatalf("participants must not edit labels: %d", w.Code)
	}
	w := f.do(t, http.MethodGet, "/api/admin/export/events.csv?group=Group%20A", admin.Token, nil)
	records, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if w.Code != http.StatusOK || err != nil || len(records) < 2 {
		t.Fatalf("export: %d %v %q", w.Code, err, w.Body.String())
	}
	if strings.Contains(strings.Join(records[0], ","), "email") || strings.Contains(w.Body.String(), "@example.com") {
		t.Fatalf("export must exclude emails unless requested: %v", records[0])
	}
	if strings.Contains(w.Body.String(), "password") {
		t.Fatal("export must never mention passwords")
	}
	w = f.do(t, http.MethodGet, "/api/admin/export/users.csv?include_email=true", admin.Token, nil)
	if !strings.Contains(w.Body.String(), "@example.com") || strings.Contains(w.Body.String(), "$2a$") {
		t.Fatalf("explicit email export must include emails and never password hashes")
	}

	// Timeline drill-down and the user's own label is not exposed to them.
	if w := f.do(t, http.MethodGet, path+"/timeline", admin.Token, nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "explain_submitted") {
		t.Fatalf("timeline: %d %s", w.Code, w.Body.String())
	}

	// Delete my analytics data.
	if w := f.do(t, http.MethodDelete, "/api/users/me/analytics", participant.Token, nil); w.Code != http.StatusOK {
		t.Fatalf("delete my data: %d %s", w.Code, w.Body.String())
	}
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM analytics_events WHERE user_id=$1`, participant.User.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("delete-my-data left %d events", n)
	}
}
