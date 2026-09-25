package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/selar-dev/selar-api/internal/middleware"
)

func TestCanonicalizeURLMatchesWorkerRules(t *testing.T) {
	got, err := canonicalizeURL("HTTPS://Example.COM:443?utm_source=test&keep=1#section")
	if err != nil {
		t.Fatalf("canonicalizeURL returned error: %v", err)
	}
	if want := "https://example.com/?keep=1"; got != want {
		t.Fatalf("canonicalizeURL = %q, want %q", got, want)
	}
}

func TestCanonicalizeURLKeepsIPv6AuthorityForWorker(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://[2606:4700:4700::1111]:443/article", "https://[2606:4700:4700::1111]/article"},
		{"https://[2606:4700:4700::1111]:8443/article", "https://[2606:4700:4700::1111]:8443/article"},
	}
	for _, tc := range cases {
		got, err := canonicalizeURL(tc.input)
		if err != nil {
			t.Fatalf("canonicalizeURL(%q) returned error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("canonicalizeURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCanonicalizeURLRejectsCredentials(t *testing.T) {
	if _, err := canonicalizeURL("https://user:password@example.com/article"); err == nil {
		t.Fatal("canonicalizeURL should reject URLs containing credentials")
	}
}

func TestUploadRejectsInvalidPDFBeforeCreatingRecords(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "broken.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte("not a PDF")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/documents/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(context.WithValue(
		request.Context(), middleware.UserIDKey, "11111111-1111-1111-1111-111111111111",
	))
	response := httptest.NewRecorder()

	New(nil).UploadDocument(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("UploadDocument status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("not a valid PDF")) {
		t.Fatalf("UploadDocument body = %q", response.Body.String())
	}
}
