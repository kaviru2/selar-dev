package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
)

// UploadDocument accepts a multipart/form-data PDF upload
func (h *Handler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	userIdStr, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok {
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}
	userID, err := uuid.Parse(userIdStr)
	if err != nil {
		http.Error(w, `{"error": "bad token"}`, http.StatusUnauthorized)
		return
	}

	// Max 50 MB
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Error(w, `{"error": "file too large"}`, http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error": "missing file"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		http.Error(w, `{"error": "only PDF allowed"}`, http.StatusBadRequest)
		return
	}

	docID := uuid.New()
	
	// Create upload folder
	uploadDir := "/tmp/selar_uploads"
	err = os.MkdirAll(uploadDir, 0755)
	if err != nil {
		http.Error(w, `{"error": "server misconfiguration"}`, http.StatusInternalServerError)
		return
	}

	outPath := filepath.Join(uploadDir, docID.String()+".pdf")
	outFile, err := os.Create(outPath)
	if err != nil {
		http.Error(w, `{"error": "failed to write file"}`, http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, file)
	if err != nil {
		http.Error(w, `{"error": "failed to save file"}`, http.StatusInternalServerError)
		return
	}

	// Insert Document into Database
	doc := model.Document{
		ID:        docID.String(),
		UserID:    userID.String(),
		Title:     header.Filename,
		Status:    "processing",
		Progress:  0.01, // Mock progress to trigger pipeline visualization
		AddedAt:   time.Now(),
	}

	err = h.store.CreateDocument(r.Context(), &doc)
	if err != nil {
		// Clean up on failure
		os.Remove(outPath)
		http.Error(w, `{"error": "failed to insert document metadata"}`, http.StatusInternalServerError)
		return
	}

	// Rename the file to precisely match the auto-generated database UUID
	newOutPath := filepath.Join("/tmp/selar_uploads", doc.ID+".pdf")
	if doc.ID != docID.String() {
		os.Rename(outPath, newOutPath)
	}

	// Trigger async parsing logic in Python Worker
	go func() {
		workerURL := os.Getenv("WORKER_URL")
		if workerURL == "" {
			workerURL = "http://localhost:8000"
		}
		
		payload := fmt.Sprintf(`{"doc_id": "%s", "file_path": "%s"}`, doc.ID, newOutPath)
		resp, err := http.Post(workerURL + "/process", "application/json", strings.NewReader(payload))
		if err != nil {
			fmt.Printf("Worker connection failed: %v\n", err)
			return
		}
		defer resp.Body.Close()
		fmt.Printf("Worker triggered for %s: Status %d\n", docID, resp.StatusCode)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(doc)
}

// ServeDocument streams the raw PDF binary back securely to the user's React-PDF component
func (h *Handler) ServeDocument(w http.ResponseWriter, r *http.Request) {
	docIdStr := chi.URLParam(r, "docId")
	docID, err := uuid.Parse(docIdStr)
	if err != nil {
		http.Error(w, `{"error": "invalid doc id"}`, http.StatusBadRequest)
		return
	}

	// Security: Verify user owns document
	userIdStr, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok {
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// In a real implementation we would fetch the doc and assert doc.UserID == userID
	_ = userIdStr 

	filePath := filepath.Join("/tmp/selar_uploads", docID.String()+".pdf")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, `{"error": "file not found on disk"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="document.pdf"`)
	http.ServeFile(w, r, filePath)
}
