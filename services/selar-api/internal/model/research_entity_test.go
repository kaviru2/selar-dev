package model

import "testing"

func TestNormalizeEntityCollapsesSymbolVariantsOnly(t *testing.T) {
	for _, alias := range []string{"τ²-bench", "𝜏2-bench", "tau2-bench", "TAU2 Bench", "τ^2-Benchmark", "Τ²-BENCH"} {
		if key, q := NormalizeEntity(alias); key != "tau2bench" || q != "" {
			t.Fatalf("%q -> %q/%q, want tau2bench", alias, key, q)
		}
	}
	// A different name must not be merged into τ²-bench: that would invent a claim.
	for _, distinct := range []string{"TAU Benchmark", "τ-bench"} {
		if key, _ := NormalizeEntity(distinct); key != "taubench" {
			t.Fatalf("%q -> %q, want taubench", distinct, key)
		}
	}
	if key, _ := NormalizeEntity("RAC"); key != "rac" {
		t.Fatalf("RAC -> %q", key)
	}
}

func TestNormalizeEntitySeparatesVersionQualifiers(t *testing.T) {
	cases := map[string][2]string{
		"SagaLLM":                {"sagallm", ""},
		"modified SagaLLM":       {"sagallm", "modified"},
		"original SagaLLM":       {"sagallm", "original"},
		"re-implemented SagaLLM": {"sagallm", "reimplemented"},
		"Original":               {"original", ""}, // a bare word stays the entity
	}
	for input, want := range cases {
		key, q := NormalizeEntity(input)
		if key != want[0] || q != want[1] {
			t.Fatalf("%q -> %q/%q, want %q/%q", input, key, q, want[0], want[1])
		}
	}
}

func TestResearchPredicateOntologyCoversIssue9Relations(t *testing.T) {
	for _, p := range []string{"uses_benchmark", "evaluated_on", "evaluates", "compared_against", "compares", "reimplemented_as", "reports_result_for", "cites"} {
		if _, ok := ResearchPredicates[p]; !ok {
			t.Fatalf("missing predicate %s", p)
		}
	}
	if _, ok := ResearchPredicates["related_to"]; ok {
		t.Fatal("generic related_to must not be a research assertion predicate")
	}
}
