package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIntegrationLockedCohortCannotUseKnownWitnessID(t *testing.T) {
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
	f := seedSuggestionGateFixture(t, pool, "control", "locked-direct-"+time.Now().Format("150405.000000000")+"@example.invalid")
	defer pool.Exec(ctx, "DELETE FROM users WHERE id=$1", f.userID)
	_, err = pool.Exec(ctx, `INSERT INTO cohort_setting_locks(cohort,setting_key,value,reason) VALUES('control','suggestions.show_on_open','false','direct-gate') ON CONFLICT(cohort,setting_key) DO UPDATE SET value='false',reason='direct-gate'`)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM cohort_setting_locks WHERE reason='direct-gate'`)
	h := New(store.New(pool))
	for _, fn := range []http.HandlerFunc{h.PreviewMentalModelLink, h.RespondToMentalModelLink, h.FlagMentalModelLink, h.RespondToSuggestion} {
		req := authed(httptest.NewRequest("POST", "/known-id", strings.NewReader(`{"action":"rejected","revision":0}`)), f.userID)
		route := chi.NewRouteContext()
		route.URLParams.Add("id", f.docID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
		rec := httptest.NewRecorder()
		fn(rec, req)
		if rec.Code != 403 {
			t.Fatalf("gate returned %d: %s", rec.Code, rec.Body.String())
		}
	}
}
