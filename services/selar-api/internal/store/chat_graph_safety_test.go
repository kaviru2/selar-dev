package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/model"
)

// The fabricated citation pair deliberately has no reviewed relationship.
// It is evidence that each concept was retrieved, not an assertion between them.
func TestChatGraphSafetyFeedbackAndLegacyProjection(t *testing.T) {
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
	var owner, other, docA, docB, chunkA, chunkB, conceptA, conceptB, thread, message string
	q := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner = q(`INSERT INTO users(email,password_hash) VALUES ($1,'test') RETURNING id`, "graph-safety-"+time.Now().Format("20060102150405.000000000")+"@example.test")
	other = q(`INSERT INTO users(email,password_hash) VALUES ($1,'test') RETURNING id`, "graph-safety-other-"+time.Now().Format("20060102150405.000000000")+"@example.test")
	defer func() {
		for _, id := range []string{owner, other} {
			if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	}()
	docA = q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'A','ready') RETURNING id`, owner)
	docB = q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'B','ready') RETURNING id`, owner)
	chunkA = q(`INSERT INTO chunks(document_id,user_id,chunk_index,content) VALUES ($1,$2,0,'Alpha appears in this passage.') RETURNING id`, docA, owner)
	chunkB = q(`INSERT INTO chunks(document_id,user_id,chunk_index,content) VALUES ($1,$2,0,'Beta appears in another passage.') RETURNING id`, docB, owner)
	conceptA = q(`INSERT INTO concepts(user_id,name,state) VALUES ($1,'Alpha','confirmed') RETURNING id`, owner)
	conceptB = q(`INSERT INTO concepts(user_id,name,state) VALUES ($1,'Beta','confirmed') RETURNING id`, owner)
	for _, pair := range [][2]string{{chunkA, conceptA}, {chunkB, conceptB}} {
		if _, err := pool.Exec(ctx, `INSERT INTO chunk_concepts(chunk_id,concept_id) VALUES ($1,$2)`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	thread = q(`INSERT INTO chat_threads(user_id) VALUES ($1) RETURNING id`, owner)
	question := q(`INSERT INTO chat_messages(thread_id,user_id,role,content) VALUES ($1,$2,'user','Compare Alpha and Beta') RETURNING id`, thread, owner)
	message = q(`INSERT INTO chat_messages(thread_id,user_id,role,content) VALUES ($1,$2,'assistant','A cited answer.') RETURNING id`, thread, owner)
	for rank, chunk := range []string{chunkA, chunkB} {
		if _, err := pool.Exec(ctx, `INSERT INTO message_citations(message_id,chunk_id,rank,score,quote) VALUES ($1,$2,$3,1,'cited passage')`, message, chunk, rank+1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO retrieval_traces(user_id,thread_id,user_message_id,assistant_message_id,query,ranking_policy,candidates) VALUES ($1,$2,$3,$4,'Compare Alpha and Beta','test','[]')`, owner, thread, question, message); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordChatFeedback(ctx, other, message, model.ChatFeedbackRequest{Action: "helpful"}); err == nil {
		t.Fatal("other owner rated an answer")
	}
	for _, action := range []string{"helpful", "unhelpful", "correction"} {
		feedback, err := s.RecordChatFeedback(ctx, owner, message, model.ChatFeedbackRequest{Action: action, CorrectionText: map[string]string{"correction": "The answer was mistaken"}[action]})
		if err != nil {
			t.Fatal(err)
		}
		if feedback.Action != action {
			t.Fatalf("saved action=%q", feedback.Action)
		}
		edges, err := s.ListConceptEdges(ctx, owner)
		if err != nil {
			t.Fatal(err)
		}
		if len(edges) != 0 {
			t.Fatalf("%s exposed unreviewed co-citation edge: %+v", action, edges)
		}
	}
	var feedbackCount, evidenceCount, success, failure int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chat_message_feedback WHERE user_id=$1`, owner).Scan(&feedbackCount); err != nil {
		t.Fatal(err)
	}
	if feedbackCount != 3 {
		t.Fatalf("feedback audit count=%d", feedbackCount)
	}
	var storedRelations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM concept_edges WHERE user_id=$1`, owner).Scan(&storedRelations); err != nil || storedRelations != 0 {
		t.Fatalf("co-citation created a stored relation: %d %v", storedRelations, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chat_concept_evidence WHERE message_id=$1`, message).Scan(&evidenceCount); err != nil {
		t.Fatal(err)
	}
	if evidenceCount != 2 {
		t.Fatalf("citation concept observations lost: %d", evidenceCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(success_count),0),COALESCE(sum(failure_count),0) FROM chat_learner_projection WHERE user_id=$1`, owner).Scan(&success, &failure); err != nil {
		t.Fatal(err)
	}
	if success != 0 || failure != 0 {
		t.Fatalf("answer ratings masquerade as retrieval outcomes: success=%d failure=%d", success, failure)
	}
	// A replay must not turn retained answer ratings into retrieval outcomes.
	if _, err := pool.Exec(ctx, `UPDATE chat_learner_projection SET success_count=1 WHERE user_id=$1`, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplayAdaptiveGraph(ctx, owner, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(success_count),0) FROM chat_learner_projection WHERE user_id=$1`, owner).Scan(&success); err != nil || success != 0 {
		t.Fatalf("replay restored a fake success: %d %v", success, err)
	}
	// Historical counters remain in the audit table, not in the chat API's live summary.
	if _, err := pool.Exec(ctx, `UPDATE chat_graph_updates SET links_observed=2,links_promoted=1 WHERE message_id=$1`, message); err != nil {
		t.Fatal(err)
	}
	messages, err := s.ListChatMessages(ctx, owner, thread)
	if err != nil || len(messages) != 3 || messages[1].GraphUpdate == nil || messages[1].GraphUpdate.LinksObserved != 0 || messages[1].GraphUpdate.LinksPromoted != 0 {
		t.Fatalf("historical relation counts leaked in chat: %+v %v", messages, err)
	}

	// Old deterministic_chat rows remain in the database as audit history, even
	// when their state says supported; only an explicit owner edge action restores visibility.
	legacy := q(`INSERT INTO concept_edges(user_id,source_concept_id,target_concept_id,relation,created_via,state) VALUES ($1,$2,$3,'related_to','deterministic_chat','supported') RETURNING id`, owner, conceptA, conceptB)
	edges, err := s.ListConceptEdges(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 0 {
		t.Fatalf("legacy chat relation exposed: %+v", edges)
	}
	var retained int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM concept_edges WHERE id=$1`, legacy).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("audit row deleted: %d %v", retained, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE concept_edges SET state='confirmed' WHERE id=$1`, legacy); err != nil {
		t.Fatal(err)
	}
	edges, err = s.ListConceptEdges(ctx, owner)
	if err != nil || len(edges) != 0 {
		t.Fatalf("legacy row without explicit review visible: %+v %v", edges, err)
	}
	created := q(`INSERT INTO concept_edges(user_id,source_concept_id,target_concept_id,relation,created_via,state) VALUES ($1,$2,$3,'prerequisite_of','user_created','confirmed') RETURNING id`, owner, conceptA, conceptB)
	edges, err = s.ListConceptEdges(ctx, owner)
	if err != nil || len(edges) != 1 || edges[0].ID != created {
		t.Fatalf("user-created link hidden: %+v %v", edges, err)
	}
	if err := s.RespondToConceptEdge(ctx, other, legacy, model.EdgeActionRequest{Action: "confirm"}); err == nil {
		t.Fatal("cross-owner edge confirmation succeeded")
	}
	if err := s.RespondToConceptEdge(ctx, owner, legacy, model.EdgeActionRequest{Action: "confirm"}); err != nil {
		t.Fatal(err)
	}
	edges, err = s.ListConceptEdges(ctx, owner)
	if err != nil || len(edges) != 2 || edges[0].ID != legacy || edges[1].ID != created {
		t.Fatalf("explicit owner review lost: %+v %v", edges, err)
	}
	edges, err = s.ListConceptEdges(ctx, other)
	if err != nil || len(edges) != 0 {
		t.Fatalf("cross-owner graph leak: %+v %v", edges, err)
	}
}
