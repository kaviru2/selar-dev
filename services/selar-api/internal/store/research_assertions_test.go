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

// Fabricated issue #9 scenario. The texts are invented; only the shape matches
// the reported failure: a later "RAC" paper evaluates RAC and a modified
// SagaLLM baseline on τ²-bench, while the original SagaLLM paper says nothing
// about that benchmark.
func TestResearchAssertionProvenanceLifecycle(t *testing.T) {
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
	owner := q(`INSERT INTO users(email,password_hash) VALUES ($1,'test') RETURNING id`, "assert-"+stamp+"@example.test")
	other := q(`INSERT INTO users(email,password_hash) VALUES ($1,'test') RETURNING id`, "assert-other-"+stamp+"@example.test")
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1)`, []string{owner, other})

	racDoc := q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'RAC paper (fabricated)','ready') RETURNING id`, owner)
	sagaDoc := q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'SagaLLM paper (fabricated)','ready') RETURNING id`, owner)
	racText := "We evaluate RAC on τ²-bench. As a baseline we re-implement a modified SagaLLM and run it on the same tasks."
	racChunk := q(`INSERT INTO chunks(document_id,user_id,chunk_index,content) VALUES ($1,$2,0,$3) RETURNING id`, racDoc, owner, racText)
	sagaChunk := q(`INSERT INTO chunks(document_id,user_id,chunk_index,content) VALUES ($1,$2,0,'SagaLLM introduces transactional planning for multi-agent workflows.') RETURNING id`, sagaDoc, owner)

	evaluatesRAC := model.ResearchAssertionInput{Subject: "RAC", Predicate: "evaluated_on", Object: "tau2-bench",
		AssertingDocumentID: racDoc, Scope: "own_work", ExperimentContext: "RAC main experiment",
		Evidence: []model.ResearchAssertionEvidence{{ChunkID: racChunk, Quote: "We evaluate RAC on τ²-bench."}}}
	baseline := model.ResearchAssertionInput{Subject: "modified SagaLLM", Predicate: "evaluated_on", Object: "τ²-bench",
		AssertingDocumentID: racDoc, Scope: "reported_about_other", ExperimentContext: "RAC comparison baseline",
		Evidence: []model.ResearchAssertionEvidence{{ChunkID: racChunk, Quote: "we re-implement a modified SagaLLM"}}}

	// Validation: unknown predicate, missing evidence, cross-document quote, foreign owner.
	bad := evaluatesRAC
	bad.Predicate = "related_to"
	if _, err := s.ProposeResearchAssertion(ctx, owner, bad); !errors.Is(err, ErrResearchAssertionInvalid) {
		t.Fatalf("generic predicate accepted: %v", err)
	}
	bad = evaluatesRAC
	bad.Evidence = nil
	if _, err := s.ProposeResearchAssertion(ctx, owner, bad); !errors.Is(err, ErrResearchAssertionInvalid) {
		t.Fatalf("assertion without evidence accepted: %v", err)
	}
	// The original SagaLLM paper cannot be made to "assert" τ²-bench with a RAC quote.
	bad = baseline
	bad.AssertingDocumentID = sagaDoc
	bad.Scope = "own_work"
	if _, err := s.ProposeResearchAssertion(ctx, owner, bad); !errors.Is(err, ErrResearchAssertionEvidence) {
		t.Fatalf("quote from another document attributed to SagaLLM: %v", err)
	}
	bad = evaluatesRAC
	bad.Evidence = []model.ResearchAssertionEvidence{{ChunkID: racChunk, Quote: "RAC was evaluated on τ²-bench."}}
	if _, err := s.ProposeResearchAssertion(ctx, owner, bad); !errors.Is(err, ErrResearchAssertionEvidence) {
		t.Fatalf("paraphrased (non-exact) quote accepted: %v", err)
	}
	if _, err := s.ProposeResearchAssertion(ctx, other, evaluatesRAC); !errors.Is(err, ErrResearchAssertionNotFound) {
		t.Fatalf("foreign owner used another user's document: %v", err)
	}
	// The DB trigger enforces the same contract even for direct writers.
	if _, err := pool.Exec(ctx, `WITH a AS (INSERT INTO research_assertions(user_id,subject,subject_key,predicate,object,object_key,asserting_document_id,scope,created_via)
		VALUES ($1,'SagaLLM','sagallm','evaluated_on','τ²-bench','tau2bench',$2,'own_work','user_proposed') RETURNING id)
		INSERT INTO research_assertion_evidence(assertion_id,chunk_id,quote,text_sha256) SELECT a.id,$3,'We evaluate RAC','x' FROM a`, owner, sagaDoc, racChunk); err == nil {
		t.Fatal("trigger allowed cross-document evidence")
	}

	rac, err := s.ProposeResearchAssertion(ctx, owner, evaluatesRAC)
	if err != nil {
		t.Fatal(err)
	}
	base, err := s.ProposeResearchAssertion(ctx, owner, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if rac.State != "proposed" || rac.ObjectKey != "tau2bench" || base.ObjectKey != "tau2bench" {
		t.Fatalf("aliases not normalized or auto-confirmed: %+v %+v", rac, base)
	}
	if base.SubjectKey != "sagallm" || base.SubjectQualifier != "modified" || base.AssertingDocument != "RAC paper (fabricated)" {
		t.Fatalf("baseline provenance lost: %+v", base)
	}
	if edges, _ := s.ListConfirmedResearchAssertionEdges(ctx, owner); len(edges) != 0 {
		t.Fatalf("proposed assertions projected into the graph: %+v", edges)
	}

	// Stale revision and foreign owner cannot review.
	if _, err := s.RespondToResearchAssertion(ctx, owner, rac.ID, model.ResearchAssertionAction{Action: "confirm", Revision: 7}); !errors.Is(err, ErrResearchAssertionStale) {
		t.Fatalf("stale confirm: %v", err)
	}
	if _, err := s.RespondToResearchAssertion(ctx, other, rac.ID, model.ResearchAssertionAction{Action: "confirm", Revision: 1}); !errors.Is(err, ErrResearchAssertionNotFound) {
		t.Fatalf("foreign confirm: %v", err)
	}
	for _, a := range []*model.ResearchAssertion{rac, base} {
		if _, err := s.RespondToResearchAssertion(ctx, owner, a.ID, model.ResearchAssertionAction{Action: "confirm", Revision: 1}); err != nil {
			t.Fatal(err)
		}
	}
	edges, err := s.ListConfirmedResearchAssertionEdges(ctx, owner)
	if err != nil || len(edges) != 2 {
		t.Fatalf("confirmed projection: %+v %v", edges, err)
	}
	// Acceptance #5: nothing in the graph says the original SagaLLM paper used τ²-bench.
	for _, e := range edges {
		if e.SubjectKey == "sagallm" && (e.AssertingDocumentID == sagaDoc || e.Scope == "own_work" || e.SubjectQualifier == "") {
			t.Fatalf("graph falsely attributes τ²-bench to original SagaLLM: %+v", e)
		}
	}
	if list, _ := s.ListResearchAssertions(ctx, owner, "", "𝜏2-bench"); len(list) != 2 {
		t.Fatalf("alias lookup should find both τ²-bench claims, got %d", len(list))
	}
	if list, _ := s.ListResearchAssertions(ctx, owner, "", "TAU Benchmark"); len(list) != 0 {
		t.Fatalf("distinct benchmark name merged into τ²-bench: %d", len(list))
	}

	// Source claims are immutable; corrections supersede.
	if _, err := pool.Exec(ctx, `UPDATE research_assertions SET subject='SagaLLM', subject_qualifier='' WHERE id=$1`, base.ID); err == nil {
		t.Fatal("source claim rewritten in place")
	}
	correction := baseline
	correction.Predicate = "reimplemented_as"
	correction.Subject = "SagaLLM"
	correction.Object = "modified SagaLLM"
	if _, err := s.RespondToResearchAssertion(ctx, owner, base.ID, model.ResearchAssertionAction{Action: "supersede", Revision: 2, Reason: "clarify baseline", Correction: &correction}); err != nil {
		t.Fatal(err)
	}
	superseded, _ := s.GetResearchAssertion(ctx, owner, base.ID)
	if superseded.State != "superseded" || superseded.SupersededBy == "" || superseded.Subject != "modified SagaLLM" {
		t.Fatalf("supersede: %+v", superseded)
	}
	replacement, _ := s.GetResearchAssertion(ctx, owner, superseded.SupersededBy)
	if replacement.State != "proposed" || replacement.Predicate != "reimplemented_as" {
		t.Fatalf("correction must await review: %+v", replacement)
	}
	if _, err := s.RespondToResearchAssertion(ctx, owner, rac.ID, model.ResearchAssertionAction{Action: "retract", Revision: 2, Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	if edges, _ := s.ListConfirmedResearchAssertionEdges(ctx, owner); len(edges) != 0 {
		t.Fatalf("retracted/superseded still projected: %+v", edges)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM research_assertion_events WHERE user_id=$1`, owner).Scan(&events); err != nil || events != 7 {
		t.Fatalf("audit events=%d %v", events, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE research_assertion_events SET reason='x' WHERE user_id=$1`, owner); err == nil {
		t.Fatal("audit events mutable")
	}

	// A changed source chunk hides evidence and blocks confirmation.
	again, err := s.ProposeResearchAssertion(ctx, owner, evaluatesRAC)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE chunks SET content = content || ' (revised)' WHERE id=$1`, racChunk); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RespondToResearchAssertion(ctx, owner, again.ID, model.ResearchAssertionAction{Action: "confirm", Revision: 1}); !errors.Is(err, ErrResearchAssertionEvidence) {
		t.Fatalf("confirmed against stale evidence: %v", err)
	}
	_ = sagaChunk
}
