package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/selar-dev/selar-api/internal/model"
)

func TestResearchAssertionGraphKeepsAssertingDocumentOnEveryEdge(t *testing.T) {
	now := time.Now()
	items := []model.ResearchAssertion{
		{ID: "a1", Subject: "RAC", SubjectKey: "rac", Predicate: "evaluated_on", Object: "τ²-bench", ObjectKey: "tau2bench",
			AssertingDocumentID: "doc-rac", AssertingDocument: "RAC paper", Scope: "own_work", State: "confirmed", CreatedAt: now,
			Evidence: []model.ResearchAssertionEvidence{{ChunkID: "c1", Quote: "We evaluate RAC on τ²-bench."}}},
		{ID: "a2", Subject: "modified SagaLLM", SubjectKey: "sagallm", SubjectQualifier: "modified", Predicate: "evaluated_on",
			Object: "tau2-bench", ObjectKey: "tau2bench", AssertingDocumentID: "doc-rac", AssertingDocument: "RAC paper",
			Scope: "reported_about_other", ExperimentContext: "comparison baseline", State: "confirmed", CreatedAt: now,
			Evidence: []model.ResearchAssertionEvidence{{ChunkID: "c1", Quote: "modified SagaLLM"}}},
		{ID: "a3", Subject: "X", SubjectKey: "x", Predicate: "cites", Object: "Y", ObjectKey: "y", State: "confirmed"}, // no live evidence
	}
	nodes, edges := researchAssertionGraph(items)
	if len(edges) != 2 {
		t.Fatalf("an assertion without live evidence was projected: %+v", edges)
	}
	ids := map[string]bool{}
	for _, n := range nodes {
		ids[n.ID] = true
	}
	// Both τ²-bench spellings share one node; modified SagaLLM is not the plain SagaLLM node.
	if !ids["entity:tau2bench"] || !ids["entity:sagallm:modified"] || ids["entity:sagallm"] || len(nodes) != 3 {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
	for _, e := range edges {
		if e.SourceDocumentID != "doc-rac" || e.AssertingDocumentTitle != "RAC paper" || e.AssertionScope == "" || e.SourceQuote == "" {
			t.Fatalf("edge lost provenance: %+v", e)
		}
	}
	if !strings.Contains(edges[1].Explanation, "another work it reports on") {
		t.Fatalf("reported-about-other scope not explained: %q", edges[1].Explanation)
	}
}

func TestResearchAssertionHandlersRejectMalformedIDsBeforeStore(t *testing.T) {
	h := &Handler{} // nil store: the handler must fail before touching it
	for _, body := range []string{
		`{"subject":"RAC","predicate":"evaluated_on","object":"x","asserting_document_id":"not-a-uuid","scope":"own_work"}`,
		`{"subject":"RAC","predicate":"evaluated_on","object":"x","asserting_document_id":"6f1c3f7e-5f6b-4b8e-9a51-1f0a3c1f2d11","scope":"own_work","evidence":[{"chunk_id":"bad","quote":"q"}]}`,
		`not json`,
	} {
		rec := httptest.NewRecorder()
		h.ProposeResearchAssertion(rec, httptest.NewRequest(http.MethodPost, "/api/research-assertions", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s -> %d", body, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ListResearchAssertions(rec, httptest.NewRequest(http.MethodGet, "/api/research-assertions?state=bogus", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus state -> %d", rec.Code)
	}
}
