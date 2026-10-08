package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/storage"
	"github.com/selar-dev/selar-api/internal/storage/storagetest"
)

const testUser = "11111111-1111-1111-1111-111111111111"

func authed(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, userID))
}

func s3Handler(t *testing.T) (*Handler, *storagetest.FakeS3) {
	t.Helper()
	fake, server := storagetest.NewFakeS3WithState(t, "selar-test")
	store, err := storage.NewS3(storage.S3Config{
		Endpoint: server.URL, Bucket: "selar-test", Region: "auto",
		AccessKeyID: "test-access", SecretAccessKey: "test-secret", ForcePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := New(nil)
	h.SetStorage(store)
	return h, fake
}

func postJSON(t *testing.T, handlerFunc http.HandlerFunc, path string, body any, userID string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	if userID != "" {
		request = authed(request, userID)
	}
	response := httptest.NewRecorder()
	handlerFunc(response, request)
	return response
}

func TestUploadURLFallsBackToMultipartForLocalStorage(t *testing.T) {
	h := New(nil)
	h.SetStorage(&storage.Local{Root: t.TempDir()})
	response := postJSON(t, h.CreateUploadURL, "/api/documents/upload-url",
		map[string]any{"filename": "notes.pdf", "size": 1024, "content_type": "application/pdf"}, testUser)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mode":"multipart"`) {
		t.Fatalf("local backend must tell the client to use multipart: %d %s", response.Code, response.Body.String())
	}
}

func TestUploadURLIssuesUserScopedPresignedPut(t *testing.T) {
	h, _ := s3Handler(t)
	response := postJSON(t, h.CreateUploadURL, "/api/documents/upload-url",
		map[string]any{"filename": "Large Reading.pdf", "size": 30 << 20, "content_type": "application/pdf"}, testUser)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Mode     string `json:"mode"`
		UploadID string `json:"upload_id"`
		Upload   struct {
			Method  string            `json:"method"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"upload"`
		MaxBytes int64 `json:"max_bytes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Mode != "direct" || payload.Upload.Method != http.MethodPut || payload.UploadID == "" {
		t.Fatalf("payload = %+v", payload)
	}
	parsed, _ := url.Parse(payload.Upload.URL)
	wantPath := "/selar-test/users/" + testUser + "/uploads/" + payload.UploadID + ".pdf"
	if parsed.Path != wantPath {
		t.Fatalf("presigned path = %q, want %q (server-generated, user-scoped)", parsed.Path, wantPath)
	}
	if expires := parsed.Query().Get("X-Amz-Expires"); expires != "600" {
		t.Fatalf("presigned URL must be short-lived, X-Amz-Expires = %q", expires)
	}
	if strings.Contains(response.Body.String(), "test-secret") {
		t.Fatal("response leaked the storage secret")
	}
}

func TestUploadURLRejectsBadRequests(t *testing.T) {
	h, _ := s3Handler(t)
	cases := []struct {
		name string
		body map[string]any
		user string
		want int
	}{
		{"unauthenticated", map[string]any{"filename": "a.pdf", "size": 10, "content_type": "application/pdf"}, "", http.StatusUnauthorized},
		{"not a uuid user", map[string]any{"filename": "a.pdf", "size": 10, "content_type": "application/pdf"}, "../../etc", http.StatusUnauthorized},
		{"wrong type", map[string]any{"filename": "a.pdf", "size": 10, "content_type": "text/html"}, testUser, http.StatusBadRequest},
		{"wrong extension", map[string]any{"filename": "a.exe", "size": 10, "content_type": "application/pdf"}, testUser, http.StatusBadRequest},
		{"empty", map[string]any{"filename": "a.pdf", "size": 0, "content_type": "application/pdf"}, testUser, http.StatusBadRequest},
		{"too large", map[string]any{"filename": "a.pdf", "size": maxPDFSize + 1, "content_type": "application/pdf"}, testUser, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := postJSON(t, h.CreateUploadURL, "/api/documents/upload-url", tc.body, tc.user)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", response.Code, tc.want, response.Body.String())
			}
		})
	}
}

func TestCompleteUploadVerifiesObjectBeforeCreatingRecords(t *testing.T) {
	// The handler has no database (store is nil): every rejection below must
	// happen before any record is created, or the test would panic.
	h, fake := s3Handler(t)
	const uploadID = "22222222-2222-2222-2222-222222222222"
	key := "users/" + testUser + "/uploads/" + uploadID + ".pdf"

	missing := postJSON(t, h.CompleteUpload, "/api/documents/upload-complete",
		map[string]any{"upload_id": uploadID, "filename": "a.pdf"}, testUser)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "not found") {
		t.Fatalf("missing object: %d %s", missing.Code, missing.Body.String())
	}

	fake.Seed(key, []byte("<html>not a pdf</html>"), "application/pdf")
	notPDF := postJSON(t, h.CompleteUpload, "/api/documents/upload-complete",
		map[string]any{"upload_id": uploadID, "filename": "a.pdf"}, testUser)
	if notPDF.Code != http.StatusBadRequest || !strings.Contains(notPDF.Body.String(), "not a valid PDF") {
		t.Fatalf("non-PDF object: %d %s", notPDF.Code, notPDF.Body.String())
	}
	if fake.Has(key) {
		t.Fatal("rejected uploads must be deleted from storage")
	}

	fake.Seed(key, []byte("%PDF-1.7 fabricated"), "text/html")
	wrongType := postJSON(t, h.CompleteUpload, "/api/documents/upload-complete",
		map[string]any{"upload_id": uploadID, "filename": "a.pdf"}, testUser)
	if wrongType.Code != http.StatusBadRequest {
		t.Fatalf("wrong stored content type: %d %s", wrongType.Code, wrongType.Body.String())
	}

	// Another user cannot finalize this user's object: the key is derived
	// from the authenticated user, so the lookup misses.
	fake.Seed(key, []byte("%PDF-1.7 fabricated"), "application/pdf")
	other := postJSON(t, h.CompleteUpload, "/api/documents/upload-complete",
		map[string]any{"upload_id": uploadID, "filename": "a.pdf"}, "33333333-3333-3333-3333-333333333333")
	if other.Code != http.StatusBadRequest || !fake.Has(key) {
		t.Fatalf("cross-user finalize must miss without deleting the owner's object: %d %s", other.Code, other.Body.String())
	}

	badID := postJSON(t, h.CompleteUpload, "/api/documents/upload-complete",
		map[string]any{"upload_id": "../../x", "filename": "a.pdf"}, testUser)
	if badID.Code != http.StatusBadRequest {
		t.Fatalf("non-UUID upload id: %d", badID.Code)
	}
}

func TestCompleteUploadRejectedForLocalStorage(t *testing.T) {
	h := New(nil)
	h.SetStorage(&storage.Local{Root: t.TempDir()})
	response := postJSON(t, h.CompleteUpload, "/api/documents/upload-complete",
		map[string]any{"upload_id": "22222222-2222-2222-2222-222222222222", "filename": "a.pdf"}, testUser)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d", response.Code)
	}
}
