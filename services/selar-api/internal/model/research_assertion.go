package model

import "time"

// ResearchPredicates is the closed ontology for provenance-aware research
// assertions (issue #9). Each describes what the asserting document claims.
var ResearchPredicates = map[string]string{
	"introduces":         "introduces",
	"uses_benchmark":     "uses benchmark",
	"evaluated_on":       "is evaluated on",
	"evaluates":          "evaluates",
	"compared_against":   "is compared against",
	"compares":           "compares",
	"reimplemented_as":   "is re-implemented as",
	"reports_result_for": "reports a result for",
	"cites":              "cites",
}

// ResearchAssertionEvidence is an exact quote from a chunk of the asserting document.
type ResearchAssertionEvidence struct {
	ChunkID string `json:"chunk_id"`
	Quote   string `json:"quote"`
	Page    int    `json:"page,omitempty"`
}

// ResearchAssertion is one document's claim with its provenance and review state.
type ResearchAssertion struct {
	ID                  string                      `json:"id"`
	Subject             string                      `json:"subject"`
	SubjectKey          string                      `json:"subject_key"`
	SubjectQualifier    string                      `json:"subject_qualifier,omitempty"`
	Predicate           string                      `json:"predicate"`
	Object              string                      `json:"object"`
	ObjectKey           string                      `json:"object_key"`
	ObjectQualifier     string                      `json:"object_qualifier,omitempty"`
	AssertingDocumentID string                      `json:"asserting_document_id"`
	AssertingDocument   string                      `json:"asserting_document_title"`
	Scope               string                      `json:"scope"`
	ExperimentContext   string                      `json:"experiment_context,omitempty"`
	Confidence          float32                     `json:"confidence"`
	CreatedVia          string                      `json:"created_via"`
	State               string                      `json:"state"`
	SupersededBy        string                      `json:"superseded_by,omitempty"`
	SourceMessageID     string                      `json:"source_message_id,omitempty"`
	Revision            int64                       `json:"revision"`
	Evidence            []ResearchAssertionEvidence `json:"evidence"`
	CreatedAt           time.Time                   `json:"created_at"`
	ReviewedAt          *time.Time                  `json:"reviewed_at,omitempty"`
}

// ResearchAssertionInput proposes a new assertion. It is never auto-confirmed.
type ResearchAssertionInput struct {
	Subject             string                      `json:"subject"`
	Predicate           string                      `json:"predicate"`
	Object              string                      `json:"object"`
	AssertingDocumentID string                      `json:"asserting_document_id"`
	Scope               string                      `json:"scope"`
	ExperimentContext   string                      `json:"experiment_context"`
	Confidence          *float32                    `json:"confidence,omitempty"`
	SourceMessageID     string                      `json:"source_message_id,omitempty"`
	Evidence            []ResearchAssertionEvidence `json:"evidence"`
}

// ResearchAssertionAction is an owner review step, bound to the revision seen.
type ResearchAssertionAction struct {
	Action     string                  `json:"action"`
	Revision   int64                   `json:"revision"`
	Reason     string                  `json:"reason"`
	Correction *ResearchAssertionInput `json:"correction,omitempty"`
}
