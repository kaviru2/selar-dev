package handler

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/store"
	"io"
	"net/http"
	"strings"
	"unicode/utf16"
)

func annotationError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "could not save annotation"
	if errors.Is(err, pgx.ErrNoRows) {
		status, message = http.StatusNotFound, "annotation or document not found"
	}
	if errors.Is(err, store.ErrAnnotationSourceChanged) {
		status, message = http.StatusConflict, err.Error()
	}
	writeJSON(w, status, map[string]string{"error": message})
}
func validAnnotationColor(c model.AnnotationColor) bool {
	return c == model.ColorWheat || c == model.ColorYellow || c == model.ColorCoral || c == model.ColorSage
}

func (h *Handler) UpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	var patch model.AnnotationPatch
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&patch); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid annotation edit"})
		return
	}
	if decoder.Decode(new(any)) != io.EOF || (patch.Color == nil && patch.Comment == nil) || (patch.Color != nil && !validAnnotationColor(*patch.Color)) || (patch.Comment != nil && len(*patch.Comment) > 10000) {
		writeJSON(w, 400, map[string]string{"error": "invalid annotation edit"})
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid annotation id"})
		return
	}
	a, err := h.store.UpdateAnnotation(r.Context(), id, middleware.GetUserID(r.Context()), patch)
	if err != nil {
		annotationError(w, err)
		return
	}
	writeJSON(w, 200, a)
}

func validNewAnnotation(a model.Annotation) bool {
	if _, err := uuid.Parse(a.DocumentID); err != nil {
		return false
	}
	if a.Page < 1 || !validAnnotationColor(a.Color) || len(a.Comment) > 10000 {
		return false
	}
	if a.Type != model.AnnotationHighlight && a.Type != model.AnnotationUnderline && a.Type != model.AnnotationNote {
		return false
	}
	if a.ChunkID != nil {
		if _, err := uuid.Parse(*a.ChunkID); err != nil {
			return false
		}
	}
	if a.Anchor != nil {
		v := a.Anchor
		if v.Version != 1 || v.SourceHash == "" || len(v.SourceHash) > 256 || strings.TrimSpace(v.Exact) == "" || len(v.Exact) > 32000 || len(v.Prefix) > 512 || len(v.Suffix) > 512 || v.Start < 0 || v.End-v.Start != len(utf16.Encode([]rune(v.Exact))) {
			return false
		}
		var boxes []struct{ X, Y, W, H float64 }
		if json.Unmarshal(a.BBox, &boxes) != nil || len(boxes) == 0 || len(boxes) > 500 {
			return false
		}
		for _, b := range boxes {
			if b.X < 0 || b.Y < 0 || b.W <= 0 || b.H <= 0 || b.X+b.W > 1.001 || b.Y+b.H > 1.001 {
				return false
			}
		}
	} else {
		// Old callers may send PDF-point rectangles; do not reinterpret their units.
		if !json.Valid(a.BBox) {
			return false
		}
	}
	return true
}
