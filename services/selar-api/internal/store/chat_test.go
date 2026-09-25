package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOnlyExplicitHelpfulFeedbackPermitsGraphReduction(t *testing.T) {
	for _, action := range []string{"", "unhelpful", "correction", "comment", "helpful "} {
		if feedbackPermitsGraphReduction(action) {
			t.Fatalf("%q must not permit a source-graph reduction", action)
		}
	}
	if !feedbackPermitsGraphReduction("helpful") {
		t.Fatal("explicit helpful feedback must permit a candidate graph reduction")
	}
}

func TestGroundedConceptExtractionRequiresQuestionAndCitationMatch(t *testing.T) {
	term, ok := conceptTermFromQuery("What is realmbench?")
	if !ok || term != "realmbench" {
		t.Fatalf("unexpected extracted term: %q, %v", term, ok)
	}
	surface, ok := groundedSurface("We evaluate the agent on REALM-Bench and other tasks.", term)
	if !ok || surface != "REALM-Bench" {
		t.Fatalf("separator-insensitive grounding failed: %q, %v", surface, ok)
	}
	if _, ok := groundedSurface("This passage discusses unrelated scheduling tasks.", term); ok {
		t.Fatal("a concept absent from cited evidence must not be grounded")
	}
	if _, ok := conceptTermFromQuery("Compare several benchmark design tradeoffs"); ok {
		t.Fatal("open-ended questions must not manufacture a concept candidate")
	}
}

func TestGroundedChatConceptDiscoveryIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	var userID, documentID, chunkID, threadID, messageID string
	if err = tx.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, 'test') RETURNING id`,
		"grounded-chat-"+time.Now().Format("20060102150405.000000000")+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `INSERT INTO documents (user_id, title, status) VALUES ($1, 'Evidence', 'ready') RETURNING id`, userID).Scan(&documentID); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `
		INSERT INTO chunks (document_id, user_id, chunk_index, content)
		VALUES ($1, $2, 0, 'REALM-Bench evaluates agents on scheduling and logistics tasks.') RETURNING id`,
		documentID, userID).Scan(&chunkID); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `INSERT INTO chat_threads (user_id) VALUES ($1) RETURNING id`, userID).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `
		INSERT INTO chat_messages (thread_id, user_id, role, content)
		VALUES ($1, $2, 'assistant', 'A grounded answer.') RETURNING id`, threadID, userID).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO message_citations (message_id, chunk_id, rank, score, quote)
		VALUES ($1, $2, 1, 1, 'REALM-Bench evaluates agents.')`, messageID, chunkID); err != nil {
		t.Fatal(err)
	}

	update, err := reduceChatGraph(ctx, tx, userID, messageID, "What is realmbench?")
	if err != nil {
		t.Fatal(err)
	}
	if update.ConceptsCreated != 1 || update.ConceptsReinforced != 0 {
		t.Fatalf("unexpected graph update: %#v", update)
	}
	var name, state, promptVersion, bindingMethod string
	if err = tx.QueryRow(ctx, `
		SELECT c.name, c.state, c.prompt_version, cce.binding_method
		FROM concepts c
		JOIN chat_concept_evidence cce ON cce.concept_id = c.id
		WHERE c.user_id = $1 AND cce.message_id = $2`, userID, messageID).
		Scan(&name, &state, &promptVersion, &bindingMethod); err != nil {
		t.Fatal(err)
	}
	if name != "REALM-Bench" || state != "candidate" || promptVersion != "grounded-chat-concept-v1" || bindingMethod != "grounded_query_exact" {
		t.Fatalf("unexpected grounded concept: %q %q %q %q", name, state, promptVersion, bindingMethod)
	}
}

func TestAdaptiveEdgeStateRequiresRepeatedOrIndependentEvidence(t *testing.T) {
	tests := []struct {
		messages  int
		documents int
		want      string
	}{
		{1, 1, "candidate"},
		{2, 1, "candidate"},
		{2, 2, "supported"},
		{3, 1, "supported"},
	}
	for _, test := range tests {
		if got := adaptiveEdgeState(test.messages, test.documents); got != test.want {
			t.Errorf("adaptiveEdgeState(%d, %d) = %q, want %q", test.messages, test.documents, got, test.want)
		}
	}
}

func TestProjectedEdgeLifecycle(t *testing.T) {
	asOf := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	recent := asOf.Add(-24 * time.Hour)
	state, confidence, validTo := projectedEdgeLifecycle("deterministic_chat", "candidate", 2, 2, &recent, asOf)
	if state != "supported" || confidence <= 0 || validTo != nil {
		t.Fatalf("recent independent evidence should remain supported: state=%s confidence=%v validTo=%v", state, confidence, validTo)
	}
	stale := asOf.Add(-91 * 24 * time.Hour)
	state, _, validTo = projectedEdgeLifecycle("deterministic_chat", "supported", 3, 2, &stale, asOf)
	if state != "archived" || validTo == nil {
		t.Fatalf("stale chat-only edge should archive: state=%s validTo=%v", state, validTo)
	}
	state, _, _ = projectedEdgeLifecycle("deterministic_chat", "confirmed", 0, 0, nil, asOf)
	if state != "confirmed" {
		t.Fatalf("human-confirmed edge must be preserved, got %s", state)
	}
}

func TestAdaptiveEdgeConfidenceIsBoundedAndMonotonic(t *testing.T) {
	baseline := adaptiveEdgeConfidence(1, 1)
	repeated := adaptiveEdgeConfidence(2, 1)
	independent := adaptiveEdgeConfidence(2, 2)
	saturated := adaptiveEdgeConfidence(100, 100)
	if baseline != 0.35 {
		t.Fatalf("baseline confidence = %v, want 0.35", baseline)
	}
	if repeated <= baseline || independent <= repeated {
		t.Fatalf("confidence must increase with evidence: %v, %v, %v", baseline, repeated, independent)
	}
	if saturated > 0.95 {
		t.Fatalf("confidence must be capped at 0.95, got %v", saturated)
	}
}

func TestReplayProjectionHashIsDeterministic(t *testing.T) {
	edges := []replayEdge{
		{id: "edge-b", expectedState: "supported", support: 3, documents: 2, expectedEvidence: 0.72},
		{id: "edge-a", expectedState: "candidate", support: 1, documents: 1, expectedEvidence: 0.35},
	}
	learners := []replayLearner{
		{conceptID: "concept-b", exposure: 2, retrieval: 1, success: 1, interest: 0.28},
		{conceptID: "concept-a", exposure: 4, retrieval: 2, failure: 1, interest: 0.35},
	}

	first := replayProjectionHash(edges, learners)
	second := replayProjectionHash(
		[]replayEdge{edges[1], edges[0]},
		[]replayLearner{learners[1], learners[0]},
	)
	if first != second {
		t.Fatalf("the same projection in a different query order produced different hashes: %s != %s", first, second)
	}

	changed := append([]replayEdge(nil), edges...)
	changed[0].support++
	if first == replayProjectionHash(changed, learners) {
		t.Fatal("a changed projection produced the same replay hash")
	}
}

func TestProjectedLifecycleReplayIsEquivalent(t *testing.T) {
	asOf := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	observed := asOf.Add(-7 * 24 * time.Hour)

	stateA, confidenceA, validToA := projectedEdgeLifecycle(
		"deterministic_chat", "candidate", 3, 2, &observed, asOf,
	)
	stateB, confidenceB, validToB := projectedEdgeLifecycle(
		"deterministic_chat", "candidate", 3, 2, &observed, asOf,
	)

	if stateA != stateB || confidenceA != confidenceB || !sameOptionalTime(validToA, validToB) {
		t.Fatalf("identical retained evidence did not replay equivalently: (%s, %v, %v) != (%s, %v, %v)",
			stateA, confidenceA, validToA, stateB, confidenceB, validToB)
	}
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
