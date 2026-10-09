package handler

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/store"
	"net/http"
)

func (h *Handler) PreviewMentalModelLink(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if !h.requireLearningLinks(w, r) {
		return
	}
	preview, err := h.store.PreviewMentalModelLink(r.Context(), middleware.GetUserID(r.Context()), id)
	if errors.Is(err, store.ErrMentalModelLinkNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "grounded assertion not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to preview assertion"})
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
