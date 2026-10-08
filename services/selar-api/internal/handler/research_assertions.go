package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/store"
)

func writeResearchAssertionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrResearchAssertionNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "assertion or document not found"})
	case errors.Is(err, store.ErrResearchAssertionStale):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "assertion changed; reload and review again"})
	case errors.Is(err, store.ErrResearchAssertionEvidence):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": store.ErrResearchAssertionEvidence.Error()})
	case errors.Is(err, store.ErrResearchAssertionInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to process research assertion"})
	}
}

// ListResearchAssertions: GET /api/research-assertions?state=&entity=
func (h *Handler) ListResearchAssertions(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state != "" && state != "proposed" && state != "confirmed" && state != "rejected" && state != "retracted" && state != "superseded" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid state"})
		return
	}
	items, err := h.store.ListResearchAssertions(r.Context(), middleware.GetUserID(r.Context()), state, strings.TrimSpace(r.URL.Query().Get("entity")))
	if err != nil {
		writeResearchAssertionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// ProposeResearchAssertion: POST /api/research-assertions. Always "proposed".
func (h *Handler) ProposeResearchAssertion(w http.ResponseWriter, r *http.Request) {
	var in model.ResearchAssertionInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if uuid.Validate(in.AssertingDocumentID) != nil || (in.SourceMessageID != "" && uuid.Validate(in.SourceMessageID) != nil) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	for _, ev := range in.Evidence {
		if uuid.Validate(ev.ChunkID) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chunk id"})
			return
		}
	}
	created, err := h.store.ProposeResearchAssertion(r.Context(), middleware.GetUserID(r.Context()), in)
	if err != nil {
		writeResearchAssertionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// RespondToResearchAssertion: POST /api/research-assertions/{id}/respond
func (h *Handler) RespondToResearchAssertion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var action model.ResearchAssertionAction
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&action); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if action.Correction != nil {
		if uuid.Validate(action.Correction.AssertingDocumentID) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}
		for _, ev := range action.Correction.Evidence {
			if uuid.Validate(ev.ChunkID) != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chunk id"})
				return
			}
		}
	}
	updated, err := h.store.RespondToResearchAssertion(r.Context(), middleware.GetUserID(r.Context()), id, action)
	if err != nil {
		writeResearchAssertionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// researchAssertionGraph projects confirmed assertions as entity nodes and
// predicate edges. Each edge names its asserting document and scope, so the
// graph never states "X used Y" without saying who reported it.
func researchAssertionGraph(items []model.ResearchAssertion) ([]model.GraphNode, []model.GraphEdge) {
	nodes := []model.GraphNode{}
	edges := []model.GraphEdge{}
	seen := map[string]bool{}
	node := func(key, qualifier, name string, a model.ResearchAssertion) string {
		id := "entity:" + key
		if qualifier != "" {
			id += ":" + strings.ReplaceAll(qualifier, " ", "-")
		}
		if !seen[id] {
			seen[id] = true
			nodes = append(nodes, model.GraphNode{ID: id, Name: name, NodeType: "entity", State: "confirmed",
				Description: "Named in confirmed research assertions.", CreatedAt: a.CreatedAt})
		}
		return id
	}
	for _, a := range items {
		if len(a.Evidence) == 0 {
			continue
		}
		scope := "its own work"
		if a.Scope == "reported_about_other" {
			scope = "another work it reports on"
		}
		explanation := a.AssertingDocument + " asserts this about " + scope + "."
		if a.ExperimentContext != "" {
			explanation += " Context: " + a.ExperimentContext + "."
		}
		observed := a.CreatedAt
		if a.ReviewedAt != nil {
			observed = *a.ReviewedAt
		}
		edges = append(edges, model.GraphEdge{
			ID: "assertion:" + a.ID, Source: node(a.SubjectKey, a.SubjectQualifier, a.Subject, a),
			Target: node(a.ObjectKey, a.ObjectQualifier, a.Object, a), Relation: a.Predicate,
			State: a.State, Confidence: a.Confidence, CreatedVia: "user_reviewed_assertion",
			Explanation: explanation, ObservedAt: &observed, ValidFrom: &observed,
			AssertionID: a.ID, AssertionScope: a.Scope, SourceDocumentID: a.AssertingDocumentID,
			AssertingDocumentTitle: a.AssertingDocument, SourceQuote: a.Evidence[0].Quote,
			ReviewRevision: a.Revision,
		})
	}
	return nodes, edges
}
