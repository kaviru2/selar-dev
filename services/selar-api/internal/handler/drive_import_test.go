package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/selar-dev/selar-api/internal/storage"
	"github.com/selar-dev/selar-api/internal/storage/storagetest"
	"github.com/selar-dev/selar-api/internal/store"
)

const driveFileID = "1AbCdEfGhIjKlMnOpQrStUvWxYz_-0123"
const driveToken = "ya29.synthetic-token"

// fakeDrive serves metadata and media for one file and records requests.
func fakeDrive(t *testing.T, name, mime string, content []byte, status int) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		if r.Header.Get("Authorization") != "Bearer "+driveToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/"+driveFileID) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("alt") == "media" {
			_, _ = w.Write(content)
			return
		}
		_, _ = w.Write([]byte(`{"name":"` + name + `","mimeType":"` + mime + `","size":"` + strconv.Itoa(len(content)) + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func driveBody(id, token string) map[string]any {
	return map[string]any{"file_id": id, "access_token": token}
}

func TestDriveImportRejectsBeforeTouchingStore(t *testing.T) {
	pdf := []byte("%PDF-1.7 synthetic")
	cases := []struct {
		name   string
		srv    func() *httptest.Server
		body   map[string]any
		status int
	}{
		{"missing token", nil, driveBody(driveFileID, ""), http.StatusBadRequest},
		{"path traversal id", nil, driveBody("../../about?x=1", driveToken), http.StatusBadRequest},
		{"token with whitespace", nil, driveBody(driveFileID, "a b"), http.StatusBadRequest},
		{"not a PDF", func() *httptest.Server {
			s, _ := fakeDrive(t, "notes.xlsx", "application/vnd.google-apps.spreadsheet", pdf, 0)
			return s
		}, driveBody(driveFileID, driveToken), http.StatusBadRequest},
		{"bad magic", func() *httptest.Server {
			s, _ := fakeDrive(t, "x.pdf", "application/pdf", []byte("<html>not a pdf"), 0)
			return s
		}, driveBody(driveFileID, driveToken), http.StatusBadRequest},
		{"expired token", func() *httptest.Server { s, _ := fakeDrive(t, "x.pdf", "application/pdf", pdf, 0); return s }, driveBody(driveFileID, "ya29.other"), http.StatusUnauthorized},
		{"not shared with the app", func() *httptest.Server {
			s, _ := fakeDrive(t, "x.pdf", "application/pdf", pdf, http.StatusNotFound)
			return s
		}, driveBody(driveFileID, driveToken), http.StatusNotFound},
		{"drive outage", func() *httptest.Server {
			s, _ := fakeDrive(t, "x.pdf", "application/pdf", pdf, http.StatusInternalServerError)
			return s
		}, driveBody(driveFileID, driveToken), http.StatusBadGateway},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := New(nil) // nil store: reaching persistence would panic
			if c.srv != nil {
				h.SetDriveAPIBase(c.srv().URL)
			} else {
				h.SetDriveAPIBase("http://127.0.0.1:1") // must not be called
			}
			rec := postJSON(t, h.ImportDriveFile, "/api/documents/import/drive", c.body, testUser)
			if rec.Code != c.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, c.status, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), driveToken) {
				t.Fatal("response echoes the access token")
			}
		})
	}
}

func TestDriveImportRejectsOversizedFileWithoutDownloading(t *testing.T) {
	var media bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") == "media" {
			media = true
		}
		_, _ = w.Write([]byte(`{"name":"big.pdf","mimeType":"application/pdf","size":"` + strconv.FormatInt(maxPDFSize+1, 10) + `"}`))
	}))
	defer srv.Close()
	h := New(nil)
	h.SetDriveAPIBase(srv.URL)
	rec := postJSON(t, h.ImportDriveFile, "/api/documents/import/drive", driveBody(driveFileID, driveToken), testUser)
	if rec.Code != http.StatusRequestEntityTooLarge || media {
		t.Fatalf("status %d, downloaded=%v", rec.Code, media)
	}
}

func TestDriveImportRequiresAuthenticatedUser(t *testing.T) {
	h := New(nil)
	if rec := postJSON(t, h.ImportDriveFile, "/api/documents/import/drive", driveBody(driveFileID, driveToken), ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

// TestIntegrationDriveImportQueuesIngestion runs the import against Postgres,
// a fake Drive and the in-process fake S3: the PDF is stored and queued like
// any upload.
func TestIntegrationDriveImportQueuesIngestion(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var owner string
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash)
		VALUES (gen_random_uuid()::text||'@example.invalid','x') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, owner)

	fakeS3, server := storagetest.NewFakeS3WithState(t, "selar-test")
	objects, err := storage.NewS3(storage.S3Config{Endpoint: server.URL, Bucket: "selar-test", Region: "auto",
		AccessKeyID: "test-access", SecretAccessKey: "test-secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("%PDF-1.7 synthetic drive fixture")
	drive, seen := fakeDrive(t, "Week 3 reading", "application/pdf", content, 0)
	h := New(store.New(pool))
	h.SetStorage(objects)
	h.SetDriveAPIBase(drive.URL)

	rec := postJSON(t, h.ImportDriveFile, "/api/documents/import/drive", driveBody(driveFileID, driveToken), owner)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var docID, filePath, status, title, sourceType string
	if err := pool.QueryRow(ctx, `SELECT d.id, j.file_path, j.status, d.title, d.source_type FROM documents d
		JOIN ingestion_jobs j ON j.document_id = d.id WHERE d.user_id = $1`, owner).Scan(&docID, &filePath, &status, &title, &sourceType); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || title != "Week 3 reading.pdf" || sourceType != "pdf" || filePath != "s3://selar-test/"+docID+".pdf" {
		t.Fatalf("job status=%q title=%q source_type=%q file_path=%q", status, title, sourceType, filePath)
	}
	if !fakeS3.Has(docID + ".pdf") {
		t.Fatalf("PDF not stored; keys %v", fakeS3.Keys())
	}
	if len(*seen) != 2 || !strings.Contains((*seen)[0], "fields=") || !strings.Contains((*seen)[1], "alt=media") {
		t.Fatalf("drive requests %v", *seen)
	}
	var tokenLeak bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ingestion_jobs WHERE user_id = $1 AND
		(file_path LIKE '%ya29%' OR title LIKE '%ya29%'))`, owner).Scan(&tokenLeak); err != nil || tokenLeak {
		t.Fatalf("access token persisted: %v %v", tokenLeak, err)
	}
}
