package handler

import (
	"bytes"
	"encoding/json"
	"github.com/selar-dev/selar-api/internal/middleware"
	"io"
	"net/http"
	"os"
	"time"
)

// Practice binds the owner server-side. This private RPC never uses quiz routes.
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
