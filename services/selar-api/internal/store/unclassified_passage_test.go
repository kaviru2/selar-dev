package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/model"
)

// A similarity-only passage match (relation='unclassified') may be dismissed
// but never confirmed or relabeled into a relation. The refusal must be a
// typed error so the HTTP layer can answer 4xx, and dismissal must not touch
// learner state or the concept graph.
func TestGroundedReviewUnclassifiedPassageMatchDismissOnly(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := New(pool)
	q := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stamp := time.Now().Format("20060102150405.000000000")
	owner := q(`INSERT INTO users(email,password_hash) VALUES ($1,'test') RETURNING id`, "unclassified-"+stamp+"@example.test")
	other := q(`INSERT INTO users(email,password_hash) VALUES ($1,'test') RETURNING id`, "unclassified-other-"+stamp+"@example.test")
	defer pool.Exec(ctx, `DELETE FROM users WHERE id=$1 OR id=$2`, owner, other)

	docA := q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'How to do Research (synthetic)','ready') RETURNING id`, owner)
	docB := q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'Methods notes (synthetic)','ready') RETURNING id`, owner)
	chunkA := q(`INSERT INTO chunks(document_id,user_id,chunk_index,content,page_start) VALUES ($1,$2,0,'Compare our solution with alternative solutions.',18) RETURNING id`, docA, owner)
	chunkB := q(`INSERT INTO chunks(document_id,user_id,chunk_index,content,page_start) VALUES ($1,$2,0,'Experiments compare a method against baselines.',3) RETURNING id`, docB, owner)
	conceptA := q(`INSERT INTO concepts(user_id,name,state) VALUES ($1,'Solution','confirmed') RETURNING id`, owner)
	conceptB := q(`INSERT INTO concepts(user_id,name,state) VALUES ($1,'Baseline','confirmed') RETURNING id`, owner)
	for _, pair := range [][2]string{{chunkA, conceptA}, {chunkB, conceptB}} {
		if _, err := pool.Exec(ctx, `INSERT INTO chunk_concepts(chunk_id,concept_id) VALUES ($1,$2)`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	suggestion := q(`INSERT INTO link_suggestions(user_id,source_chunk_id,target_chunk_id,similarity,relation,status)
		VALUES ($1,$2,$3,0.74,'unclassified','pending') RETURNING id`, owner, chunkA, chunkB)

	sideEffects := func() (learner, edges, events int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM learner_concept_state WHERE user_id=$1),
			(SELECT count(*) FROM concept_edges WHERE user_id=$1),
			(SELECT count(*) FROM learning_events WHERE user_id=$1 AND suggestion_id=$2)`, owner, suggestion).
			Scan(&learner, &edges, &events); err != nil {
			t.Fatal(err)
		}
		return
	}
	status := func() string {
		return q(`SELECT status FROM link_suggestions WHERE id=$1`, suggestion)
	}

	for _, action := range []model.SuggestionStatus{model.SuggestionConfirmed, model.SuggestionRelabeled} {
		err := s.RespondToSuggestion(ctx, owner, suggestion, action, "extends", 12345)
		if !errors.Is(err, ErrUnclassifiedNotConfirmable) {
			t.Fatalf("%s on unclassified match: want ErrUnclassifiedNotConfirmable, got %v", action, err)
		}
	}
	if got := status(); got != "pending" {
		t.Fatalf("refused confirm must leave match pending, got %q", got)
	}
	if l, e, ev := sideEffects(); l != 0 || e != 0 || ev != 0 {
		t.Fatalf("refused confirm wrote side effects: learner=%d edges=%d events=%d", l, e, ev)
	}

	if err := s.RespondToSuggestion(ctx, other, suggestion, model.SuggestionRejected, "", 10); !errors.Is(err, ErrSuggestionNotFound) {
		t.Fatalf("foreign dismiss: want ErrSuggestionNotFound, got %v", err)
	}
	if got := status(); got != "pending" {
		t.Fatalf("foreign dismiss changed status to %q", got)
	}

	if err := s.RespondToSuggestion(ctx, owner, suggestion, model.SuggestionRejected, "", 123456); err != nil {
		t.Fatalf("owner dismiss must succeed: %v", err)
	}
	if got := status(); got != "rejected" {
		t.Fatalf("dismiss status = %q", got)
	}
	if l, e, ev := sideEffects(); l != 0 || e != 0 || ev != 1 {
		t.Fatalf("dismiss side effects: learner=%d edges=%d events=%d (want 0,0,1)", l, e, ev)
	}
	// Retrying a dismissal is idempotent, not an error.
	if err := s.RespondToSuggestion(ctx, owner, suggestion, model.SuggestionRejected, "", 1); err != nil {
		t.Fatalf("repeat dismiss: %v", err)
	}
	if _, _, ev := sideEffects(); ev != 1 {
		t.Fatalf("repeat dismiss duplicated learning event: %d", ev)
	}
}
