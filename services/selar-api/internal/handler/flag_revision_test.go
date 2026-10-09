package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/store"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIntegrationFlagCannotRetractNewerDisplayedRevision(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires DB")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	f := seedSuggestionGateFixture(t, pool, "treatment_hitl", "flag-"+time.Now().Format("150405.000000000")+"@example.invalid")
	defer pool.Exec(ctx, "DELETE FROM users WHERE id=$1", f.userID)
	var id string
	pool.QueryRow(ctx, "SELECT id FROM mental_model_links WHERE user_id=$1", f.userID).Scan(&id)
	_, err = pool.Exec(ctx, "UPDATE mental_model_links SET review_revision=1 WHERE id=$1", id)
	if err != nil {
		t.Fatal(err)
	}
	h := New(store.New(pool))
	req := authed(httptest.NewRequest("POST", "/flag", strings.NewReader(`{"revision":0}`)), f.userID)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	rec := httptest.NewRecorder()
	h.FlagMentalModelLink(rec, req)
	if rec.Code != 409 {
		t.Fatalf("stale flag = %d %s", rec.Code, rec.Body.String())
	}
	var count int
	pool.QueryRow(ctx, "SELECT count(*) FROM mental_link_review_events WHERE user_id=$1", f.userID).Scan(&count)
	if count != 0 {
		t.Fatal("stale flag changed history")
	}
}
