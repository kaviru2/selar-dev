package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/model"
)

// Fabricated fixture for issue #9: a later "Birch" comparison paper cites a
// "Cedar" system. Negative feedback on an answer must retract every adaptive
// effect that answer could have had, and a later helpful click on the same
// answer must not resurrect it.
type rollbackFixture struct {
	pool                          *pgxpool.Pool
	s                             *Store
	owner, thread, chunk, concept string
	unrelatedChunk, nearConcept   string
}

func newRollbackFixture(t *testing.T) (*rollbackFixture, func()) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	f := &rollbackFixture{pool: pool, s: New(pool)}
	q := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.owner = q(`INSERT INTO users(email,password_hash) VALUES ($1,'test') RETURNING id`,
		"rollback-"+time.Now().Format("20060102150405.000000000")+"@example.test")
	doc := q(`INSERT INTO documents(user_id,title,status) VALUES ($1,'Birch comparison (fabricated)','ready') RETURNING id`, f.owner)
	// Vector with a single non-zero dimension keeps cosine similarity exact.
	vec := func(dim int) string {
		out := "["
		for i := 0; i < 3072; i++ {
			if i > 0 {
				out += ","
			}
			if i == dim {
				out += "1"
			} else {
				out += "0"
			}
		}
		return out + "]"
	}
	f.chunk = q(`INSERT INTO chunks(document_id,user_id,chunk_index,content,embedding) VALUES ($1,$2,0,'Birch reports a modified Cedar baseline on BeaconBench.',$3::vector) RETURNING id`, doc, f.owner, vec(0))
	f.unrelatedChunk = q(`INSERT INTO chunks(document_id,user_id,chunk_index,content,embedding) VALUES ($1,$2,1,'Workflow diagrams for network construction.',$3::vector) RETURNING id`, doc, f.owner, vec(1))
	f.concept = q(`INSERT INTO concepts(user_id,name,state) VALUES ($1,'BeaconBench','confirmed') RETURNING id`, f.owner)
	// An unrelated concept whose embedding is identical to the unrelated chunk:
	// embedding proximity alone must never bind it to a cited passage.
	f.nearConcept = q(`INSERT INTO concepts(user_id,name,state,embedding) VALUES ($1,'Agentic Recommender','confirmed',$2::vector) RETURNING id`, f.owner, vec(1))
	if _, err := pool.Exec(ctx, `INSERT INTO chunk_concepts(chunk_id,concept_id) VALUES ($1,$2)`, f.chunk, f.concept); err != nil {
		t.Fatal(err)
	}
	f.thread = q(`INSERT INTO chat_threads(user_id) VALUES ($1) RETURNING id`, f.owner)
	return f, func() {
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, f.owner); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		pool.Close()
	}
}

func (f *rollbackFixture) answer(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	var question, message string
	if err := f.pool.QueryRow(ctx, `INSERT INTO chat_messages(thread_id,user_id,role,content) VALUES ($1,$2,'user','Did Cedar use BeaconBench?') RETURNING id`, f.thread, f.owner).Scan(&question); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO chat_messages(thread_id,user_id,role,content) VALUES ($1,$2,'assistant','Cedar used BeaconBench [S1].') RETURNING id`, f.thread, f.owner).Scan(&message); err != nil {
		t.Fatal(err)
	}
	for rank, chunk := range []string{f.chunk, f.unrelatedChunk} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO message_citations(message_id,chunk_id,rank,score,quote) VALUES ($1,$2,$3,1,'q')`, message, chunk, rank+1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO retrieval_traces(user_id,thread_id,user_message_id,assistant_message_id,query,ranking_policy,candidates) VALUES ($1,$2,$3,$4,'Did Cedar use BeaconBench?','test','[]')`, f.owner, f.thread, question, message); err != nil {
		t.Fatal(err)
	}
	return message
}

type adaptiveState struct {
	activeEvidence, fallbackEvidence, exposure, learnerEvidence int
	interest                                                    float64
}

func (f *rollbackFixture) state(t *testing.T) adaptiveState {
	t.Helper()
	ctx := context.Background()
	var s adaptiveState
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE cce.active), count(*) FILTER (WHERE cce.binding_method='embedding_fallback')
		FROM chat_concept_evidence cce JOIN chat_messages m ON m.id=cce.message_id WHERE m.user_id=$1`, f.owner).Scan(&s.activeEvidence, &s.fallbackEvidence); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT COALESCE(sum(exposure_count),0), COALESCE(sum(interest_score),0) FROM chat_learner_projection WHERE user_id=$1`, f.owner).Scan(&s.exposure, &s.interest); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT COALESCE(sum(evidence_count),0) FROM learner_concept_state WHERE user_id=$1`, f.owner).Scan(&s.learnerEvidence); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestChatFeedbackRollbackUnhelpfulRetractsHelpfulEvidence(t *testing.T) {
	f, done := newRollbackFixture(t)
	defer done()
	ctx := context.Background()
	message := f.answer(t)
	if _, err := f.s.RecordChatFeedback(ctx, f.owner, message, model.ChatFeedbackRequest{Action: "helpful"}); err != nil {
		t.Fatal(err)
	}
	after := f.state(t)
	if after.activeEvidence != 1 || after.exposure != 1 || after.learnerEvidence != 1 {
		t.Fatalf("helpful should record exactly one explicit citation observation: %+v", after)
	}
	if after.fallbackEvidence != 0 {
		t.Fatalf("embedding proximity bound an unrelated concept to a cited passage: %+v", after)
	}
	if _, err := f.s.RecordChatFeedback(ctx, f.owner, message, model.ChatFeedbackRequest{Action: "unhelpful"}); err != nil {
		t.Fatal(err)
	}
	retracted := f.state(t)
	if retracted.activeEvidence != 0 || retracted.exposure != 0 || retracted.interest > 1e-6 || retracted.learnerEvidence != 0 {
		t.Fatalf("unhelpful left adaptive evidence active: %+v", retracted)
	}
	var events int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM learning_events WHERE user_id=$1 AND chat_message_id=$2 AND event_type='chat_graph_evidence_retracted'`, f.owner, message).Scan(&events); err != nil || events != 1 {
		t.Fatalf("retraction audit event=%d %v", events, err)
	}
	var retained int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM chat_concept_evidence WHERE message_id=$1 AND NOT active AND superseded_at IS NOT NULL`, message).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("retracted evidence must remain as audit history: %d %v", retained, err)
	}
	if _, err := f.s.ReplayAdaptiveGraph(ctx, f.owner, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if replayed := f.state(t); replayed.activeEvidence != 0 || replayed.exposure != 0 {
		t.Fatalf("replay resurrected retracted evidence: %+v", replayed)
	}
}

func TestChatFeedbackRollbackHelpfulAfterNegativeDoesNotReinforce(t *testing.T) {
	f, done := newRollbackFixture(t)
	defer done()
	ctx := context.Background()
	for _, first := range []model.ChatFeedbackRequest{{Action: "unhelpful"}, {Action: "correction", CorrectionText: "That was a modified baseline in a later paper."}} {
		message := f.answer(t)
		if _, err := f.s.RecordChatFeedback(ctx, f.owner, message, first); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.RecordChatFeedback(ctx, f.owner, message, model.ChatFeedbackRequest{Action: "helpful"}); err != nil {
			t.Fatal(err)
		}
		if got := f.state(t); got.activeEvidence != 0 || got.exposure != 0 || got.learnerEvidence != 0 {
			t.Fatalf("helpful after %s reinforced a rejected answer: %+v", first.Action, got)
		}
	}
}
