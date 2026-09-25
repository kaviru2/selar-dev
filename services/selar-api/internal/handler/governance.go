package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
)

func (h *Handler) DeleteChatThread(w http.ResponseWriter, r *http.Request) {
	err := h.store.SoftDeleteChatThread(r.Context(), middleware.GetUserID(r.Context()), chi.URLParam(r, "id"))
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete conversation"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "retention_policy": "retain_evidence"})
}

func (h *Handler) RecordCitationOpen(w http.ResponseWriter, r *http.Request) {
	recorded, err := h.store.RecordCitationOpen(r.Context(), middleware.GetUserID(r.Context()), chi.URLParam(r, "id"))
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "citation not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record citation interaction"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": recorded})
}

func (h *Handler) RecordChatFeedback(w http.ResponseWriter, r *http.Request) {
	var request model.ChatFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	request.Action = strings.TrimSpace(request.Action)
	request.CorrectionText = strings.TrimSpace(request.CorrectionText)
	if request.Action != "helpful" && request.Action != "unhelpful" && request.Action != "correction" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be helpful, unhelpful, or correction"})
		return
	}
	if request.Action == "correction" && (request.CorrectionText == "" || len(request.CorrectionText) > 2000) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "correction must contain 1 to 2000 characters"})
		return
	}
	if request.Action != "correction" && len(request.CorrectionText) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feedback comment must be at most 2000 characters"})
		return
	}
	feedback, err := h.store.RecordChatFeedback(r.Context(), middleware.GetUserID(r.Context()), chi.URLParam(r, "id"), request)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "message not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record feedback"})
		return
	}
	writeJSON(w, http.StatusCreated, feedback)
}

func (h *Handler) RespondToConceptEdge(w http.ResponseWriter, r *http.Request) {
	var request model.EdgeActionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	request.Action = strings.TrimSpace(request.Action)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Action != "confirm" && request.Action != "reject" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be confirm or reject"})
		return
	}
	if len(request.Reason) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason must be at most 500 characters"})
		return
	}
	err := h.store.RespondToConceptEdge(r.Context(), middleware.GetUserID(r.Context()), chi.URLParam(r, "id"), request)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "relationship not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update relationship"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": request.Action})
}

func (h *Handler) RespondToConcept(w http.ResponseWriter, r *http.Request) {
	var request model.EdgeActionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	request.Action = strings.TrimSpace(request.Action)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Action != "confirm" && request.Action != "reject" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be confirm or reject"})
		return
	}
	if len(request.Reason) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason must be at most 500 characters"})
		return
	}
	if err := h.store.RespondToConcept(r.Context(), middleware.GetUserID(r.Context()), chi.URLParam(r, "id"), request); err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "candidate concept not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update concept"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": request.Action})
}

func (h *Handler) RecordLearnerSignal(w http.ResponseWriter, r *http.Request) {
	var request model.LearnerSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	request.ConceptID = strings.TrimSpace(request.ConceptID)
	request.Signal = strings.TrimSpace(request.Signal)
	request.IdempotencyID = strings.TrimSpace(request.IdempotencyID)
	if request.ConceptID == "" || request.IdempotencyID == "" ||
		(request.Signal != "quiz_success" && request.Signal != "quiz_failure") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "concept_id, supported signal, and idempotency_id are required"})
		return
	}
	// The prototype has no validated quiz attempt/response workflow. Client-supplied
	// outcomes must not become learner evidence even for an owned concept.
	writeJSON(w, http.StatusConflict, map[string]string{"error": "assessment signals are unavailable until validated quiz attempts exist"})
}

func (h *Handler) ReplayAdaptiveGraph(w http.ResponseWriter, r *http.Request) {
	request := model.ReplayRequest{}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&request)
	}
	report, err := h.store.ReplayAdaptiveGraph(r.Context(), middleware.GetUserID(r.Context()), request.Apply, time.Now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to replay adaptive graph"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) ApplyGraphLifecycle(w http.ResponseWriter, r *http.Request) {
	report, err := h.store.ReplayAdaptiveGraph(r.Context(), middleware.GetUserID(r.Context()), true, time.Now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to apply graph lifecycle"})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) GetEvaluationMetrics(w http.ResponseWriter, r *http.Request) {
	summary, err := h.store.GetMetricsSummary(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load evaluation metrics"})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
