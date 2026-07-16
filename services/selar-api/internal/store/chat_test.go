package store

import "testing"

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
