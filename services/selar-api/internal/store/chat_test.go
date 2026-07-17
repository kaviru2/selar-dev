package store

import "testing"
import "time"

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
