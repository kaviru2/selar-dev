package handler

import (
	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
	"net/http"
)

func (h *Handler) requireLearningLinks(w http.ResponseWriter, r *http.Request) bool {
	allowed, err := h.suggestionsEnabled(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "learning prompts unavailable"})
		return false
	}
	if !allowed {
		writeJSON(w, 403, map[string]string{"error": "learning prompts disabled for this cohort"})
		return false
	}
	return true
}

// Flag uses the existing revision-locked audit/retraction transaction. It never
// confirms a relationship, creates an edge, or increments recall success.
func (h *Handler) FlagMentalModelLink(w http.ResponseWriter, r *http.Request) {
	if !h.requireLearningLinks(w, r) {
		return
	}
	owner := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")
	preview, err := h.store.PreviewMentalModelLink(r.Context(), owner, id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "live prompt not found"})
		return
	}
	if preview.Status == model.MentalLinkRejected || preview.Status == model.MentalLinkArchived {
		writeJSON(w, 200, map[string]string{"status": "hidden"})
		return
	}
	action := model.MentalLinkRejected
	if preview.Status == model.MentalLinkConfirmed || preview.Status == model.MentalLinkRelabeled {
		action = "retracted"
	}
	err = h.store.RespondToMentalModelLink(r.Context(), owner, id, model.MentalModelLinkResponse{Action: action, Revision: &preview.Revision, Reason: "Learner flagged: This link is wrong"})
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "prompt changed; refresh"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "hidden"})
}
