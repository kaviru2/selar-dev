package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/storage"
	"github.com/selar-dev/selar-api/internal/storage/storagetest"
	"github.com/selar-dev/selar-api/internal/store"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func formatStore(t *testing.T) (*Handler, *pgxpool.Pool, string) {
	t.Helper()
	db := os.Getenv("TEST_DATABASE_URL")
	if db == "" {
		t.Skip("requires isolated TEST_DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var owner string
	if err := pool.QueryRow(context.Background(), `INSERT INTO users(email,password_hash) VALUES(gen_random_uuid()::text||'@example.invalid','x') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, owner) })
	h := New(store.New(pool))
	h.SetStorage(&storage.Local{Root: t.TempDir()})
	return h, pool, owner
}

func TestIntegrationTextFilesQueueOriginal(t *testing.T) {
	h, pool, owner := formatStore(t)
	for _, filename := range []string{"notes.md", "notes.txt"} {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, _ := form.CreateFormFile("file", filename)
		data := []byte("# Original\n\nco- operation must remain distinct.")
		part.Write(data)
		form.Close()
		req := authed(httptest.NewRequest("POST", "/api/documents/upload", &body), owner)
		req.Header.Set("Content-Type", form.FormDataContentType())
		rec := httptest.NewRecorder()
		h.UploadDocument(rec, req)
		if rec.Code != 202 {
			t.Fatalf("%s: %d %s", filename, rec.Code, rec.Body.String())
		}
		var result struct {
			Document struct {
				ID string `json:"id"`
			} `json:"document"`
		}
		json.Unmarshal(rec.Body.Bytes(), &result)
		var snapshotHash string
		if err := pool.QueryRow(context.Background(), `SELECT metadata->>'original_sha256' FROM documents WHERE id=$1`, result.Document.ID).Scan(&snapshotHash); err != nil || len(snapshotHash) != 64 {
			t.Fatalf("missing original snapshot hash: %v %s", err, snapshotHash)
		}
		var kind, locator, status string
		err := pool.QueryRow(context.Background(), `SELECT source_type,file_path,status FROM ingestion_jobs WHERE document_id=$1`, result.Document.ID).Scan(&kind, &locator, &status)
		if err != nil || status != "queued" {
			t.Fatalf("job %v %s", err, status)
		}
		if kind == "pdf" {
			t.Fatal("text must not masquerade as PDF")
		}
		stored, err := os.ReadFile(locator)
		if err != nil || !bytes.Equal(stored, data) {
			t.Fatalf("original bytes: %v", err)
		}
	}
}

func syntheticDOCX(extra string) []byte {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	parts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Fabricated evidence.</w:t></w:r></w:p></w:body></w:document>`,
	}
	if extra != "" {
		parts[extra] = strings.Repeat("A", 100000)
	}
	for name, data := range parts {
		file, _ := archive.Create(name)
		file.Write([]byte(data))
	}
	archive.Close()
	return buf.Bytes()
}

func TestDOCXStructuralValidation(t *testing.T) {
	if err := validateFile(syntheticDOCX(""), "docx"); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"word/vbaProject.bin", "../escape", "word/bomb"} {
		if err := validateFile(syntheticDOCX(extra), "docx"); err == nil {
			t.Fatalf("accepted %s", extra)
		}
	}
	if err := validateFile([]byte("PK fake ZIP"), "docx"); err == nil {
		t.Fatal("accepted ZIP magic alone")
	}
}

func TestIntegrationDirectFormatFiles(t *testing.T) {
	h, pool, owner := formatStore(t)
	fake, server := storagetest.NewFakeS3WithState(t, "selar-test")
	objects, err := storage.NewS3(storage.S3Config{Endpoint: server.URL, Bucket: "selar-test", Region: "auto", AccessKeyID: "test-access", SecretAccessKey: "test-secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	h.SetStorage(objects)
	for _, item := range []struct {
		name, mime string
		data       []byte
	}{{"paper.pdf", "application/pdf", []byte("%PDF-1.7 immutable synthetic")}, {"notes.md", "text/markdown", []byte("# Original evidence")}, {"notes.txt", "text/plain", []byte("Literal evidence")}, {"notes.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", syntheticDOCX("")}} {
		const id = "55555555-4444-4444-4444-444444444444"
		key, _ := storage.UserUploadKey(owner, id)
		fake.Seed(key, item.data, item.mime)
		rec := postJSON(t, h.CompleteUpload, "/api/documents/upload-complete", map[string]any{"upload_id": id, "filename": item.name}, owner)
		if rec.Code != 202 {
			t.Fatalf("%s: %d %s", item.name, rec.Code, rec.Body.String())
		}
		var result struct {
			Document struct {
				ID string `json:"id"`
			} `json:"document"`
		}
		json.Unmarshal(rec.Body.Bytes(), &result)
		var locator string
		if err := pool.QueryRow(context.Background(), `SELECT file_path FROM ingestion_jobs WHERE document_id=$1`, result.Document.ID).Scan(&locator); err != nil {
			t.Fatal(err)
		}
		fake.Seed(key, []byte("reused staging PUT"), item.mime)
		reader, _, err := objects.Open(context.Background(), locator)
		if err != nil {
			t.Fatal(err)
		}
		frozen, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || !bytes.Equal(frozen, item.data) {
			t.Fatalf("%s snapshot changed with staging upload", item.name)
		}
	}
}

func TestTextFileUploadPlan(t *testing.T) {
	h := New(nil)
	for _, item := range []struct{ name, mime string }{{"notes.md", "text/markdown"}, {"notes.txt", "text/plain"}} {
		rec := postJSON(t, h.CreateUploadURL, "/api/documents/upload-url", map[string]any{"filename": item.name, "content_type": item.mime, "size": 20}, testUser)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "multipart") || !strings.Contains(rec.Body.String(), "10485760") {
			t.Fatalf("%s: %d %s", item.name, rec.Code, rec.Body.String())
		}
	}
}
