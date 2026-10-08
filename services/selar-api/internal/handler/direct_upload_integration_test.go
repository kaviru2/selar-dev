package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/selar-dev/selar-api/internal/storage"
	"github.com/selar-dev/selar-api/internal/storage/storagetest"
	"github.com/selar-dev/selar-api/internal/store"
	"github.com/selar-dev/selar-api/internal/workertrigger"
)

// TestIntegrationDirectUploadQueuesStorageLocatorAndOwnerOnlyDelete runs the
// complete direct-upload flow against Postgres and the in-process fake S3.
func TestIntegrationDirectUploadQueuesStorageLocatorAndOwnerOnlyDelete(t *testing.T) {
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
	var owner, intruder string
	for _, id := range []*string{&owner, &intruder} {
		if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash)
			VALUES (gen_random_uuid()::text||'@example.invalid','x') RETURNING id`).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1 OR id = $2`, owner, intruder)

	fake, server := storagetest.NewFakeS3WithState(t, "selar-test")
	objects, err := storage.NewS3(storage.S3Config{Endpoint: server.URL, Bucket: "selar-test", Region: "auto",
		AccessKeyID: "test-access", SecretAccessKey: "test-secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	h := New(store.New(pool))
	h.SetStorage(objects)
	triggered := make(chan string, 1)
	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/jobs/trigger" && r.Header.Get(workertrigger.SecretHeader) == "integration-secret-value" {
			triggered <- body["job_id"]
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer workerServer.Close()
	h.SetWorkerTrigger(workertrigger.FromEnv(func(key string) string {
		return map[string]string{"WORKER_URL": workerServer.URL, "WORKER_TRIGGER_SECRET": "integration-secret-value"}[key]
	}))

	const uploadID = "44444444-4444-4444-4444-444444444444"
	key := "users/" + owner + "/uploads/" + uploadID + ".pdf"
	fake.Seed(key, []byte("%PDF-1.7 fabricated synthetic fixture"), "application/pdf")
	response := postJSON(t, h.CompleteUpload, "/api/documents/upload-complete",
		map[string]any{"upload_id": uploadID, "filename": "../Synthetic Reading.pdf"}, owner)
	if response.Code != http.StatusAccepted {
		t.Fatalf("complete status = %d body = %s", response.Code, response.Body.String())
	}
	var docID, jobID, filePath, status, title string
	if err := pool.QueryRow(ctx, `SELECT d.id, j.id, j.file_path, j.status, d.title FROM documents d
		JOIN ingestion_jobs j ON j.document_id = d.id WHERE d.user_id = $1`, owner).Scan(&docID, &jobID, &filePath, &status, &title); err != nil {
		t.Fatal(err)
	}
	if filePath != "s3://selar-test/"+key || status != "queued" || title != "Synthetic Reading.pdf" {
		t.Fatalf("job file_path=%q status=%q title=%q", filePath, status, title)
	}
	select {
	case got := <-triggered:
		if got != jobID {
			t.Fatalf("worker triggered for %q, want committed job %q", got, jobID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker was not triggered after the job was queued")
	}

	serve := func(userID string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/api/documents/"+docID+"/pdf", nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("docId", docID)
		routeCtx.URLParams.Add("id", docID)
		request = authed(request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeCtx)), userID)
		recorder := httptest.NewRecorder()
		h.ServeDocument(recorder, request)
		return recorder
	}
	if got := serve(intruder); got.Code != http.StatusNotFound {
		t.Fatalf("intruder must not read the PDF: %d", got.Code)
	}
	owned := serve(owner)
	if owned.Code != http.StatusFound || !strings.Contains(owned.Header().Get("Location"), "/selar-test/"+key) ||
		!strings.Contains(owned.Header().Get("Location"), "X-Amz-Expires=300") {
		t.Fatalf("owner should get a short-lived presigned redirect: %d %q", owned.Code, owned.Header().Get("Location"))
	}

	remove := func(userID string) int {
		request := httptest.NewRequest(http.MethodDelete, "/api/documents/"+docID, nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", docID)
		request = authed(request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeCtx)), userID)
		recorder := httptest.NewRecorder()
		h.DeleteDocument(recorder, request)
		return recorder.Code
	}
	if code := remove(intruder); code != http.StatusNotFound || !fake.Has(key) {
		t.Fatalf("intruder delete must not touch storage: code=%d stillStored=%v", code, fake.Has(key))
	}
	if code := remove(owner); code != http.StatusOK || fake.Has(key) {
		t.Fatalf("owner delete must remove the object: code=%d stillStored=%v", code, fake.Has(key))
	}
}
