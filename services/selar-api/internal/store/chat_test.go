package store

import (
	"testing"
	"time"
)

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
