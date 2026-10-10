package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/selar-dev/selar-api/internal/middleware"
)

var practiceUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Practice binds the owner server-side. This private RPC never uses quiz routes.
//
// Read-only actions (daily, progress, items) and generate calls for a document
// that already has live items are answered from the API's own database pool.
// Only AI work (first-time generation, grading an attempt) goes to the worker,
// so opening /review, /progress or the reader never waits on Modal.
func (h *Handler) Practice(w http.ResponseWriter, r *http.Request) {
	owner := middleware.GetUserID(r.Context())
	if owner == "" {
		writeJSON(w, 401, map[string]string{"error": "authentication required"})
		return
	}
	var data map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 20000)).Decode(&data); err != nil || data == nil {
		writeJSON(w, 400, map[string]string{"error": "invalid practice request"})
		return
	}
	data["user_id"] = owner
	// Practice data is private to its owner: never let a CDN or shared cache keep it.
	w.Header().Set("Cache-Control", "private, no-store")
	if h.store != nil && h.practiceFromStore(w, r, owner, data) {
		return
	}
	body, err := json.Marshal(data)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	url := os.Getenv("WORKER_URL")
	if url == "" {
		url = "http://localhost:8000"
	}
	req, err := http.NewRequestWithContext(r.Context(), "POST", url+"/practice", bytes.NewReader(body))
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "practice unavailable"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if h.worker != nil {
		h.worker.Authorize(req)
	}
	res, err := (&http.Client{Timeout: 115 * time.Second}).Do(req)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "practice unavailable; retry"})
		return
	}
	defer res.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(res.Body, 100000))
}

// practiceFromStore answers read-only practice actions directly. It returns
// false when the request must go to the worker: AI work, or a database error
// (the worker stays the fallback path).
func (h *Handler) practiceFromStore(w http.ResponseWriter, r *http.Request, owner string, data map[string]any) bool {
	action, _ := data["action"].(string)
	documentID, _ := data["document_id"].(string)
	ctx := r.Context()
	switch action {
	case "daily":
		queue, err := h.store.PracticeDailyQueue(ctx, owner)
		if err != nil {
			log.Printf("practice daily from store failed; using worker: %v", err)
			return false
		}
		writeJSON(w, http.StatusOK, queue)
		return true
	case "progress":
		report, err := h.store.PracticeProgressReport(ctx, owner)
		if err != nil {
			log.Printf("practice progress from store failed; using worker: %v", err)
			return false
		}
		writeJSON(w, http.StatusOK, report)
		return true
	case "items", "generate":
		if !practiceUUID.MatchString(documentID) {
			return false // the worker applies its own validation
		}
		items, err := h.store.PracticeDocumentItems(ctx, owner, documentID)
		if err != nil {
			log.Printf("practice items from store failed; using worker: %v", err)
			return false
		}
		if action == "generate" && len(items) == 0 {
			return false // nothing live yet: the worker generates (AI) under its lock
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "items": items})
		return true
	}
	return false
}
