package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/model"
)

func reviewFixture(t *testing.T) (*Store, context.Context, string, string, string, string, func()) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires CI Postgres/pgvector TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	// Dedicated synthetic owner; deletion cascades all fixture rows.
	var owner, intruder, srcDoc, tgtDoc, srcChunk, tgtChunk, srcModel, tgtModel, link string
	q := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner = q(`INSERT INTO users(email,password_hash) VALUES (gen_random_uuid()::text||'@example.invalid','x') RETURNING id`)
	cleanup := func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1 OR id=$2`, owner, intruder); pool.Close() }
	intruder = q(`INSERT INTO users(email,password_hash) VALUES (gen_random_uuid()::text||'@example.invalid','x') RETURNING id`)
	srcDoc = q(`INSERT INTO documents(user_id,title,content_hash,status) VALUES ($1,'new','snapshot-new','ready') RETURNING id`, owner)
	tgtDoc = q(`INSERT INTO documents(user_id,title,content_hash,status) VALUES ($1,'old','snapshot-old','ready') RETURNING id`, owner)
	srcText := "The paper discusses gradient descent optimization."
	tgtText := "Gradient descent optimization is compared with momentum."
	srcChunk = q(`INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) VALUES ($1,$2,0,$3,'{"page":2}') RETURNING id`, owner, srcDoc, srcText)
	tgtChunk = q(`INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) VALUES ($1,$2,0,$3,'{"page":3}') RETURNING id`, owner, tgtDoc, tgtText)
	srcModel = q(`INSERT INTO document_mental_models(user_id,document_id,main_claim,key_concepts) VALUES ($1,$2,'claim',ARRAY['gradient descent optimization']) RETURNING id`, owner, srcDoc)
	tgtModel = q(`INSERT INTO document_mental_models(user_id,document_id,main_claim,key_concepts) VALUES ($1,$2,'claim',ARRAY['gradient descent optimization']) RETURNING id`, owner, tgtDoc)
	witness := func(doc, chunk, hash, text, quote string, page int) []byte {
		sum := sha256.Sum256([]byte(text))
		result, _ := json.Marshal(map[string]any{"chunk_id": chunk, "asserting_source_id": doc, "source_snapshot_hash": hash, "text_sha256": hex.EncodeToString(sum[:]), "quote": quote, "asserted_concept": "gradient descent optimization", "locator": map[string]int{"page": page}})
		return result
	}
	link = q(`INSERT INTO mental_model_links(user_id,source_model_id,target_model_id,link_type,source_evidence_chunk_id,target_evidence_chunk_id,source_evidence,target_evidence) VALUES ($1,$2,$3,'concept_overlap',$4,$5,$6,$7) RETURNING id`, owner, srcModel, tgtModel, srcChunk, tgtChunk, witness(srcDoc, srcChunk, "snapshot-new", srcText, "gradient descent optimization", 2), witness(tgtDoc, tgtChunk, "snapshot-old", tgtText, "Gradient descent optimization", 3))
	return New(pool), ctx, owner, intruder, link, tgtDoc, cleanup
}

func TestGroundedReviewPreviewIsOwnerScopedAndSideEffectFree(t *testing.T) {
	s, ctx, owner, intruder, id, _, cleanup := reviewFixture(t)
	defer cleanup()
	preview, err := s.PreviewMentalModelLink(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Revision != 0 || preview.Status != model.MentalLinkCandidate || preview.SourceQuote == "" || preview.TargetQuote == "" {
		t.Fatalf("bad preview: %+v", preview)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mental_link_review_events WHERE link_id=$1`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("preview wrote history: %d %v", count, err)
	}
	if _, err := s.PreviewMentalModelLink(ctx, intruder, id); err != ErrMentalModelLinkNotFound {
		t.Fatalf("cross-owner preview: %v", err)
	}
}

func TestReviewTransitionRejectsUnsupportedRelabelAndRepeatedDecision(t *testing.T) {
	if _, err := planGroundedReview(model.MentalLinkCandidate, nil, model.MentalModelLinkResponse{Action: model.MentalLinkRelabeled, Label: "claim_extension"}); err == nil {
		t.Fatal("unsupported relationship relabel was accepted")
	}
	if _, err := planGroundedReview(model.MentalLinkConfirmed, nil, model.MentalModelLinkResponse{Action: model.MentalLinkConfirmed}); err == nil {
		t.Fatal("repeated confirmation accepted")
	}
	if _, err := planGroundedReview(model.MentalLinkCandidate, nil, model.MentalModelLinkResponse{Action: model.MentalLinkConfirmed}); err != nil {
		t.Fatal(err)
	}
}
