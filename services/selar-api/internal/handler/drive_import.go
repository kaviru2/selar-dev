package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/selar-dev/selar-api/internal/storage"
)

// DefaultDriveAPIBase is the Google Drive v3 files endpoint.
const DefaultDriveAPIBase = "https://www.googleapis.com/drive/v3/files"

// Drive file ids are URL-safe base64-like strings; anything else is refused
// before a request is built, so the id cannot alter the request path.
var driveFileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{10,200}$`)

// SetDriveAPIBase overrides the Drive endpoint (tests).
func (h *Handler) SetDriveAPIBase(base string) { h.driveAPIBase = strings.TrimRight(base, "/") }

func (h *Handler) driveBase() string {
	if h.driveAPIBase != "" {
		return h.driveAPIBase
	}
	return DefaultDriveAPIBase
}

type driveImportRequest struct {
	FileID      string `json:"file_id"`
	AccessToken string `json:"access_token"`
}

type driveFileMeta struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     string `json:"size"`
}

var driveHTTP = &http.Client{Timeout: 60 * time.Second}

// ImportDriveFile imports one PDF the learner picked with Google Picker.
// The browser holds a short-lived access token with the drive.file scope
// (access only to files the user picked for SELAR); it is used here for two
// requests (metadata, then the bytes) and is never stored or logged. The
// PDF then follows the same path as an upload: size limit, PDF magic bytes,
// storage, and an ingestion job.
func (h *Handler) ImportDriveFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	var req driveImportRequest
	if err := decodeStrict(r, &req); err != nil || req.FileID == "" || req.AccessToken == "" {
		badRequest(w, "file_id and access_token are required")
		return
	}
	if !driveFileIDPattern.MatchString(req.FileID) {
		badRequest(w, "invalid file_id")
		return
	}
	if len(req.AccessToken) > 4096 || strings.ContainsAny(req.AccessToken, " \r\n\t") {
		badRequest(w, "invalid access_token")
		return
	}

	meta, status, err := h.driveMetadata(r.Context(), req.FileID, req.AccessToken)
	if err != nil {
		log.Printf("drive import: metadata: %v", err)
		writeJSON(w, status, map[string]string{"error": driveErrorMessage(status)})
		return
	}
	native := meta.MimeType == "application/vnd.google-apps.document"
	if meta.MimeType != pdfContentType && !native {
		badRequest(w, "Only PDF files and native Google Docs can be imported from Google Drive")
		return
	}
	size, sizeErr := strconv.ParseInt(meta.Size, 10, 64)
	if !native && (sizeErr != nil || size <= 0) {
		badRequest(w, "The Drive file is empty")
		return
	}
	limit := maxPDFSize
	if native {
		limit = 10 << 20
	}
	if !native && size > limit {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file too large"})
		return
	}

	var body io.ReadCloser
	if native {
		body, status, err = h.driveExport(r.Context(), req.FileID, req.AccessToken)
	} else {
		body, status, err = h.driveDownload(r.Context(), req.FileID, req.AccessToken)
	}
	if err != nil {
		writeJSON(w, status, map[string]string{"error": driveErrorMessage(status)})
		return
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not read Drive snapshot"})
		return
	}
	if int64(len(data)) > limit {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file too large"})
		return
	}
	if !native && int64(len(data)) != size {
		badRequest(w, "Drive file size changed; select it again")
		return
	}
	if !bytes.HasPrefix(data, pdfMagic) {
		badRequest(w, "file is not a valid PDF")
		return
	}

	title := cleanPDFTitle(meta.Name)
	if !strings.HasSuffix(strings.ToLower(title), ".pdf") {
		title = cleanPDFTitle(title + ".pdf")
	}
	metadata, _ := json.Marshal(map[string]any{"provider": "google_drive", "provider_file_id": req.FileID,
		"provider_mime_type": meta.MimeType, "original_filename": meta.Name, "imported_at": time.Now().UTC().Format(time.RFC3339Nano),
		"snapshot": true, "sync": false, "export_mime_type": pdfContentType, "export_sha256": fmt.Sprintf("%x", sha256.Sum256(data))})
	h.createFileIngestion(w, r, userID, title, "pdf", pdfContentType, metadata, func(ctx context.Context, documentID string) (string, error) {
		return h.storage.Put(ctx, storage.PDFKey(documentID), bytes.NewReader(data), int64(len(data)), pdfContentType)
	})
}

func (h *Handler) driveExport(ctx context.Context, fileID, token string) (io.ReadCloser, int, error) {
	u := h.driveBase() + "/" + url.PathEscape(fileID) + "/export?" + url.Values{"mimeType": {pdfContentType}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := driveHTTP.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, driveStatus(res.StatusCode), fmt.Errorf("drive export status %d", res.StatusCode)
	}
	return res.Body, http.StatusOK, nil
}

func driveErrorMessage(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "Google Drive access expired. Please pick the file again."
	case http.StatusNotFound:
		return "SELAR can't see that Drive file. Please pick it again."
	default:
		return "Google Drive didn't return the file. Please try again."
	}
}

// driveStatus maps a Drive response status to the status SELAR returns.
func driveStatus(code int) int {
	switch code {
	case http.StatusUnauthorized:
		return http.StatusUnauthorized
	case http.StatusForbidden, http.StatusNotFound:
		return http.StatusNotFound
	default:
		return http.StatusBadGateway
	}
}

func (h *Handler) driveRequest(ctx context.Context, fileID, token string, query url.Values) (*http.Response, error) {
	u := h.driveBase() + "/" + url.PathEscape(fileID) + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return driveHTTP.Do(req)
}

func (h *Handler) driveMetadata(ctx context.Context, fileID, token string) (*driveFileMeta, int, error) {
	res, err := h.driveRequest(ctx, fileID, token, url.Values{"fields": {"name,mimeType,size"}, "supportsAllDrives": {"true"}})
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, driveStatus(res.StatusCode), fmt.Errorf("drive metadata status %d", res.StatusCode)
	}
	var meta driveFileMeta
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&meta); err != nil {
		return nil, http.StatusBadGateway, err
	}
	return &meta, http.StatusOK, nil
}

func (h *Handler) driveDownload(ctx context.Context, fileID, token string) (io.ReadCloser, int, error) {
	res, err := h.driveRequest(ctx, fileID, token, url.Values{"alt": {"media"}, "supportsAllDrives": {"true"}})
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, driveStatus(res.StatusCode), fmt.Errorf("drive download status %d", res.StatusCode)
	}
	return res.Body, http.StatusOK, nil
}
