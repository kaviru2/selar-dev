package handler

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
)

const (
	maxPDFSize       int64 = 50 << 20
	maxMultipartBody int64 = maxPDFSize + (1 << 20)
	multipartMemory  int64 = 8 << 20
)

// UploadDocument accepts a multipart/form-data PDF upload
func (h *Handler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	userIdStr, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok {
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if _, err := uuid.Parse(userIdStr); err != nil {
		http.Error(w, `{"error": "bad token"}`, http.StatusUnauthorized)
		return
	}

	// Cap the entire request, while letting PDFs larger than 8 MB spill to a
	// temporary file instead of retaining the whole upload in process memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartBody)
	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		http.Error(w, `{"error": "file too large"}`, http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

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
	if header.Size > maxPDFSize {
		http.Error(w, `{"error": "file too large"}`, http.StatusRequestEntityTooLarge)
		return
	}

	// Create upload folder
	uploadDir := "/tmp/selar_uploads"
	err = os.MkdirAll(uploadDir, 0755)
	if err != nil {
		http.Error(w, `{"error": "server misconfiguration"}`, http.StatusInternalServerError)
		return
	}

	source := &model.ContentSource{
		UserID: userIdStr, Kind: "pdf", URI: header.Filename, Title: header.Filename,
		RefreshPolicy: "never",
	}
	if err := h.store.CreateSource(r.Context(), source); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create PDF source"})
		return
	}
	doc := &model.Document{
		UserID: userIdStr, Title: header.Filename, Status: model.DocStatusProcessing,
		Progress: 0.01, SourceID: &source.ID, SourceType: "pdf", MimeType: "application/pdf",
	}
	if err := h.store.CreateDocument(r.Context(), doc); err != nil {
		_ = h.store.ArchiveSource(r.Context(), source.ID, userIdStr)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create PDF snapshot"})
		return
	}
	outPath := filepath.Join(uploadDir, doc.ID+".pdf")
	outFile, err := os.Create(outPath)
	if err != nil {
		_ = h.store.DeleteDocument(r.Context(), doc.ID, userIdStr)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to write PDF"})
		return
	}
	_, copyErr := io.Copy(outFile, file)
	closeErr := outFile.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(outPath)
		_ = h.store.DeleteDocument(r.Context(), doc.ID, userIdStr)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save PDF"})
		return
	}
	run := &model.IngestionRun{
		SourceID: source.ID, DocumentID: doc.ID,
		EmbeddingModel: "gemini-embedding-2", EmbeddingDimension: 3072,
	}
	if err := h.store.CreateIngestionRun(r.Context(), run, userIdStr); err != nil {
		_ = os.Remove(outPath)
		_ = h.store.DeleteDocument(r.Context(), doc.ID, userIdStr)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to queue PDF ingestion"})
		return
	}
	go triggerWorker(workerProcessRequest{
		DocumentID: doc.ID, SourceID: source.ID, RunID: run.ID,
		SourceType: "pdf", FilePath: outPath, Title: source.Title,
	})
	writeJSON(w, http.StatusAccepted, map[string]any{"source": source, "document": doc, "run": run})
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

	if _, err := h.store.GetDocument(r.Context(), docID.String(), userIdStr); err != nil {
		http.Error(w, `{"error": "document not found"}`, http.StatusNotFound)
		return
	}

	filePath := filepath.Join("/tmp/selar_uploads", docID.String()+".pdf")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, `{"error": "file not found on disk"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="document.pdf"`)
	http.ServeFile(w, r, filePath)
}
