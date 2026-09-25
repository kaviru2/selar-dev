package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/model"
)

const maxPastedTextBytes = 2 << 20

type addContentRequest struct {
	SourceType string `json:"source_type"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Text       string `json:"text"`
}

type workerProcessRequest struct {
	DocumentID string `json:"doc_id"`
	SourceID   string `json:"source_id"`
	RunID      string `json:"run_id"`
	SourceType string `json:"source_type"`
	FilePath   string `json:"file_path,omitempty"`
	SourceURL  string `json:"source_url,omitempty"`
	RawText    string `json:"raw_text,omitempty"`
	Title      string `json:"title,omitempty"`
}

func canonicalizeURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("invalid URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("only http and https URLs are supported")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("URLs containing credentials are not supported")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	port := parsed.Port()
	if port != "" && !((parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443")) {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	parsed.Host = host
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.Fragment = ""
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || lower == "fbclid" || lower == "gclid" {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (h *Handler) AddContent(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req addContentRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPastedTextBytes+64*1024))
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or oversized request body"})
		return
	}

	req.SourceType = strings.ToLower(strings.TrimSpace(req.SourceType))
	if req.SourceType != "web" && req.SourceType != "text" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_type must be web or text"})
		return
	}

	source := &model.ContentSource{UserID: userID, Kind: req.SourceType, Title: strings.TrimSpace(req.Title)}
	process := workerProcessRequest{SourceType: req.SourceType}
	if req.SourceType == "web" {
		canonical, err := canonicalizeURL(req.URL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if existing, err := h.store.FindSourceByCanonicalURI(r.Context(), userID, canonical); err == nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":     "this source is already in your library; refresh it instead",
				"source_id": existing.ID,
			})
			return
		}
		source.URI = strings.TrimSpace(req.URL)
		source.CanonicalURI = canonical
		if source.Title == "" {
			source.Title = parsedHost(canonical)
		}
		process.SourceURL = canonical
	} else {
		process.RawText = strings.TrimSpace(req.Text)
		if process.RawText == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
			return
		}
		if len(process.RawText) > maxPastedTextBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "text exceeds 2 MB"})
			return
		}
		if source.Title == "" {
			source.Title = "Pasted notes"
		}
		source.RefreshPolicy = "never"
	}

	if err := h.store.CreateSource(r.Context(), source); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create content source"})
		return
	}
	doc, run, err := h.createIngestionSnapshot(r, source, process)
	if err != nil {
		_ = h.store.ArchiveSource(r.Context(), source.ID, userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to queue ingestion"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"source": source, "document": doc, "run": run})
}

func parsedHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "Web article"
	}
	return strings.TrimPrefix(parsed.Hostname(), "www.")
}

func (h *Handler) createIngestionSnapshot(r *http.Request, source *model.ContentSource, process workerProcessRequest) (*model.Document, *model.IngestionRun, error) {
	doc := &model.Document{
		UserID: source.UserID, Title: source.Title, Status: model.DocStatusProcessing,
		Progress: 0.01, SourceID: &source.ID, SourceType: source.Kind,
		SourceURL: source.URI, CanonicalURL: source.CanonicalURI,
	}
	if source.Kind == "web" {
		doc.MimeType = "text/html"
	} else if source.Kind == "text" {
		doc.MimeType = "text/plain"
	}
	if err := h.store.CreateDocument(r.Context(), doc); err != nil {
		return nil, nil, err
	}
	run := &model.IngestionRun{
		SourceID: source.ID, DocumentID: doc.ID,
		EmbeddingModel: "gemini-embedding-2", EmbeddingDimension: 3072,
	}
	if err := h.store.CreateIngestionRun(r.Context(), run, source.UserID); err != nil {
		_ = h.store.DeleteDocument(r.Context(), doc.ID, source.UserID)
		return nil, nil, err
	}
	process.DocumentID = doc.ID
	process.SourceID = source.ID
	process.RunID = run.ID
	process.Title = source.Title
	job := &model.IngestionJob{
		RunID: run.ID, SourceID: source.ID, DocumentID: doc.ID, UserID: source.UserID,
		SourceType: process.SourceType, FilePath: process.FilePath, SourceURL: process.SourceURL,
		RawText: process.RawText, Title: process.Title,
	}
	if err := h.store.CreateIngestionJob(r.Context(), job); err != nil {
		_ = h.store.DeleteDocument(r.Context(), doc.ID, source.UserID)
		return nil, nil, err
	}
	return doc, run, nil
}

func (h *Handler) ListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.store.ListSources(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sources"})
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (h *Handler) ListIngestionRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.store.ListIngestionRuns(r.Context(), chi.URLParam(r, "id"), middleware.GetUserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list ingestion runs"})
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *Handler) RefreshSource(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	source, err := h.store.GetSource(r.Context(), chi.URLParam(r, "id"), userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "source not found"})
		return
	}
	if source.Kind != "web" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only web sources can be refreshed"})
		return
	}
	doc, run, err := h.createIngestionSnapshot(r, source, workerProcessRequest{SourceType: "web", SourceURL: source.CanonicalURI})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to queue refresh"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"source": source, "document": doc, "run": run})
}

func (h *Handler) RetryDocumentIngestion(w http.ResponseWriter, r *http.Request) {
	run, job, err := h.store.RetryIngestion(r.Context(), chi.URLParam(r, "id"), middleware.GetUserID(r.Context()))
	if err != nil {
		log.Printf("retry ingestion failed for document %s: %v", chi.URLParam(r, "id"), err)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "document cannot be retried while work is active or no prior ingestion payload exists"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run": run, "job": job})
}

func (h *Handler) ArchiveSource(w http.ResponseWriter, r *http.Request) {
	if err := h.store.ArchiveSource(r.Context(), chi.URLParam(r, "id"), middleware.GetUserID(r.Context())); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to archive source"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

func (h *Handler) GetDocumentContent(w http.ResponseWriter, r *http.Request) {
	content, err := h.store.GetDocumentContent(r.Context(), chi.URLParam(r, "id"), middleware.GetUserID(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "document content not found"})
		return
	}
	writeJSON(w, http.StatusOK, content)
}

func (h *Handler) ServeAsset(w http.ResponseWriter, r *http.Request) {
	asset, err := h.store.GetAsset(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "assetId"), middleware.GetUserID(r.Context()))
	if err != nil || asset.StoragePath == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", asset.MimeType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, asset.StoragePath)
}
