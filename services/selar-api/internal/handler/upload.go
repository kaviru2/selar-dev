package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/storage"
)

const (
	maxPDFSize       int64 = 50 << 20
	maxMultipartBody int64 = maxPDFSize + (1 << 20)
	multipartMemory  int64 = 8 << 20
	pdfContentType         = "application/pdf"
	directUploadTTL        = 10 * time.Minute
	assetURLTTL            = 5 * time.Minute
)

var pdfMagic = []byte("%PDF-")

func authenticatedUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return "", false
	}
	if _, err := uuid.Parse(userID); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "bad token"})
		return "", false
	}
	return userID, true
}

func cleanPDFTitle(filename string) string {
	title := strings.TrimSpace(path.Base(strings.ReplaceAll(filename, "\\", "/")))
	if title == "" || title == "." || title == "/" {
		title = "document.pdf"
	}
	if len(title) > 255 {
		title = title[:255]
	}
	return title
}

// UploadDocument accepts a multipart/form-data PDF upload. It is the path used
// with local storage (Docker Compose, local development) and remains available
// with S3 storage for small files.
func (h *Handler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUser(w, r)
	if !ok {
		return
	}

	// Cap the entire request, while letting PDFs larger than 8 MB spill to a
	// temporary file instead of retaining the whole upload in process memory.
	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartBody)
	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file too large"})
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file"})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only PDF allowed"})
		return
	}
	if header.Size > maxPDFSize {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file too large"})
		return
	}
	magic := make([]byte, len(pdfMagic))
	if _, err := io.ReadFull(file, magic); err != nil || !bytes.Equal(magic, pdfMagic) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is not a valid PDF"})
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to read PDF"})
		return
	}

	title := cleanPDFTitle(header.Filename)
	h.createPDFIngestion(w, r, userID, title, func(ctx context.Context, documentID string) (string, error) {
		return h.storage.Put(ctx, storage.PDFKey(documentID), file, header.Size, pdfContentType)
	})
}

type uploadURLRequest struct {
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

// CreateUploadURL issues a short-lived presigned PUT for a direct browser
// upload. The object key is generated here and scoped to the authenticated
// user; the declared size and content type are bound by the signature.
// With local storage it tells the client to use the multipart endpoint.
func (h *Handler) CreateUploadURL(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	var req uploadURLRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(req.Filename), ".pdf") || req.ContentType != pdfContentType {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only PDF allowed"})
		return
	}
	if req.Size <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is empty"})
		return
	}
	if req.Size > maxPDFSize {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file too large"})
		return
	}
	uploadID := uuid.NewString()
	key, err := storage.UserUploadKey(userID, uploadID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "bad token"})
		return
	}
	upload, err := h.storage.PresignPut(r.Context(), key, pdfContentType, req.Size, directUploadTTL)
	if errors.Is(err, storage.ErrDirectUploadUnsupported) {
		writeJSON(w, http.StatusOK, map[string]any{"mode": "multipart", "max_bytes": maxPDFSize})
		return
	}
	if err != nil {
		log.Printf("presign upload failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not prepare upload"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode": "direct", "upload_id": uploadID, "upload": upload, "max_bytes": maxPDFSize,
	})
}

type completeUploadRequest struct {
	UploadID string `json:"upload_id"`
	Filename string `json:"filename"`
}

// CompleteUpload registers a directly uploaded PDF. It re-derives the key
// from the authenticated user, verifies the object exists, is within the size
// limit, was stored as application/pdf and starts with the PDF magic bytes,
// and only then creates records and enqueues ingestion.
func (h *Handler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	if h.storage.Backend() != "s3" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "direct uploads are not enabled; use multipart upload"})
		return
	}
	var req completeUploadRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	key, err := storage.UserUploadKey(userID, req.UploadID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upload id"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(req.Filename), ".pdf") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only PDF allowed"})
		return
	}
	locator := h.storage.Locator(key)
	info, err := h.storage.Stat(r.Context(), locator)
	if errors.Is(err, storage.ErrNotFound) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "uploaded file not found; upload again"})
		return
	}
	if err != nil {
		log.Printf("stat direct upload failed: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not verify upload"})
		return
	}
	reject := func(status int, message string) {
		if err := h.storage.Delete(r.Context(), locator); err != nil {
			log.Printf("delete rejected upload failed: %v", err)
		}
		writeJSON(w, status, map[string]string{"error": message})
	}
	if info.Size <= 0 || info.Size > maxPDFSize {
		reject(http.StatusRequestEntityTooLarge, "file too large")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(strings.Split(info.ContentType, ";")[0]), pdfContentType) {
		reject(http.StatusBadRequest, "only PDF allowed")
		return
	}
	head, err := h.storage.ReadHead(r.Context(), locator, len(pdfMagic))
	if err != nil || !bytes.Equal(head, pdfMagic) {
		reject(http.StatusBadRequest, "file is not a valid PDF")
		return
	}
	title := cleanPDFTitle(req.Filename)
	h.createPDFIngestion(w, r, userID, title, func(context.Context, string) (string, error) {
		return locator, nil
	})
}

// createPDFIngestion creates the source, document, run and durable job, using
// place to persist (or adopt) the PDF bytes. Every failure rolls back.
func (h *Handler) createPDFIngestion(w http.ResponseWriter, r *http.Request, userID, title string,
	place func(ctx context.Context, documentID string) (string, error)) {
	ctx := r.Context()
	source := &model.ContentSource{
		UserID: userID, Kind: "pdf", URI: title, Title: title, RefreshPolicy: "never",
	}
	if err := h.store.CreateSource(ctx, source); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create PDF source"})
		return
	}
	doc := &model.Document{
		UserID: userID, Title: title, Status: model.DocStatusProcessing,
		Progress: 0.01, SourceID: &source.ID, SourceType: "pdf", MimeType: pdfContentType,
	}
	if err := h.store.CreateDocument(ctx, doc); err != nil {
		_ = h.store.ArchiveSource(ctx, source.ID, userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create PDF snapshot"})
		return
	}
	locator, err := place(ctx, doc.ID)
	rollback := func(message string) {
		if locator != "" {
			_ = h.storage.DeleteDocument(context.WithoutCancel(ctx), doc.ID, locator)
		}
		_ = h.store.DeleteDocument(ctx, doc.ID, userID)
		_ = h.store.ArchiveSource(ctx, source.ID, userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": message})
	}
	if err != nil {
		log.Printf("store PDF failed: %v", err)
		rollback("failed to save PDF")
		return
	}
	run := &model.IngestionRun{
		SourceID: source.ID, DocumentID: doc.ID,
		EmbeddingModel: "gemini-embedding-2", EmbeddingDimension: 3072,
	}
	if err := h.store.CreateIngestionRun(ctx, run, userID); err != nil {
		rollback("failed to queue PDF ingestion")
		return
	}
	job := &model.IngestionJob{
		RunID: run.ID, SourceID: source.ID, DocumentID: doc.ID, UserID: userID,
		SourceType: "pdf", FilePath: locator, Title: source.Title,
	}
	if err := h.store.CreateIngestionJob(ctx, job); err != nil {
		rollback("failed to queue PDF ingestion")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"source": source, "document": doc, "run": run})
}

// ServeDocument returns the owner's PDF. With object storage it redirects to
// a short-lived presigned GET so large files never pass through the function.
func (h *Handler) ServeDocument(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(chi.URLParam(r, "docId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid doc id"})
		return
	}
	userID, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	if _, err := h.store.GetDocument(r.Context(), docID.String(), userID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document not found"})
		return
	}
	locator := h.storage.Locator(storage.PDFKey(docID.String()))
	if job, err := h.store.GetLatestIngestionJob(r.Context(), docID.String(), userID); err == nil && job.SourceType == "pdf" && job.FilePath != "" {
		locator = job.FilePath
	}
	h.serveObject(w, r, locator, pdfContentType, "private, no-store")
}

// serveObject redirects to a presigned GET for object storage, or streams the
// local file (with Range support) for the local backend.
func (h *Handler) serveObject(w http.ResponseWriter, r *http.Request, locator, contentType, cacheControl string) {
	signed, err := h.storage.PresignGet(r.Context(), locator, assetURLTTL, contentType)
	if err == nil {
		w.Header().Set("Cache-Control", "private, no-store")
		http.Redirect(w, r, signed, http.StatusFound)
		return
	}
	if !errors.Is(err, storage.ErrDirectUploadUnsupported) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}
	reader, _, err := h.storage.Open(r.Context(), locator)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cacheControl)
	if contentType == pdfContentType {
		w.Header().Set("Content-Disposition", `inline; filename="document.pdf"`)
	}
	if seeker, ok := reader.(io.ReadSeeker); ok {
		http.ServeContent(w, r, "", time.Time{}, seeker)
		return
	}
	_, _ = io.Copy(w, reader)
}
