package handler

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/selar-dev/selar-api/internal/store"
)

// Reproduces the production report: accepting a similarity-only passage card
// returned a generic 500. Confirm/relabel must be refused with a clear 4xx,
// while dismissing must still succeed.
func TestIntegrationUnclassifiedPassageResponseStatusCodes(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stamp := time.Now().Format("20060102150405.000000000")
	owner := q(`INSERT INTO users(email,password_hash) VALUES ($1,'x') RETURNING id`, "unc-handler-"+stamp+"@example.invalid")
	defer pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
	docA := q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'A','ready') RETURNING id`, owner)
	docB := q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'B','ready') RETURNING id`, owner)
	chunkA := q(`INSERT INTO chunks(document_id,user_id,chunk_index,content) VALUES ($1,$2,0,'alpha') RETURNING id`, docA, owner)
	chunkB := q(`INSERT INTO chunks(document_id,user_id,chunk_index,content) VALUES ($1,$2,0,'beta') RETURNING id`, docB, owner)
	suggestion := q(`INSERT INTO link_suggestions(user_id,source_chunk_id,target_chunk_id,similarity,relation,status)
		VALUES ($1,$2,$3,0.74,'unclassified','pending') RETURNING id`, owner, chunkA, chunkB)

	h := New(store.New(pool))
	respond := func(id string, body map[string]any) (int, string) {
		t.Helper()
		w := postJSON(t, func(w http.ResponseWriter, r *http.Request) {
			rc := chi.NewRouteContext()
			rc.URLParams.Add("id", id)
			h.RespondToSuggestion(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc)))
		}, "/api/suggestions/"+id+"/respond", body, owner)
		return w.Code, w.Body.String()
	}

	// The exact 49-byte body the reader's "Accept" button sent in production.
	code, body := respond(suggestion, map[string]any{"action": "confirmed", "time_to_respond_ms": 12345})
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "dismiss") {
		t.Fatalf("confirm unclassified: want 422 with dismiss guidance, got %d %s", code, body)
	}
	code, body = respond(suggestion, map[string]any{"action": "relabeled", "label": "extends", "time_to_respond_ms": 1})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("relabel unclassified: want 422, got %d %s", code, body)
	}
	code, body = respond("00000000-0000-0000-0000-000000000000", map[string]any{"action": "rejected", "time_to_respond_ms": 1})
	if code != http.StatusNotFound {
		t.Fatalf("unknown suggestion: want 404, got %d %s", code, body)
	}
	code, body = respond(suggestion, map[string]any{"action": "rejected", "time_to_respond_ms": 123456})
	if code != http.StatusOK {
		t.Fatalf("dismiss unclassified: want 200, got %d %s", code, body)
	}
}
