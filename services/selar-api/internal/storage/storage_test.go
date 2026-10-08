package storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/selar-dev/selar-api/internal/storage/storagetest"
)

func cfg(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestFromEnvDefaultsToLocalUploadsDirectory(t *testing.T) {
	store, err := FromEnv(cfg(nil))
	if err != nil {
		t.Fatal(err)
	}
	local, ok := store.(*Local)
	if !ok || local.Root != "/tmp/selar_uploads" || store.Backend() != "local" {
		t.Fatalf("default store = %#v", store)
	}
}

func TestFromEnvRequiresCompleteS3ConfigWithoutEchoingSecrets(t *testing.T) {
	_, err := FromEnv(cfg(map[string]string{
		"STORAGE_BACKEND": "s3", "S3_BUCKET": "selar", "S3_SECRET_ACCESS_KEY": "super-secret-value",
	}))
	if err == nil || !strings.Contains(err.Error(), "S3_ENDPOINT") {
		t.Fatalf("expected a missing-endpoint error, got %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatal("configuration errors must not include secrets")
	}
	if _, err := FromEnv(cfg(map[string]string{"STORAGE_BACKEND": "ftp"})); err == nil {
		t.Fatal("unknown backends must fail")
	}
}

func TestLocalPutOpenStatDeleteAndPrefix(t *testing.T) {
	ctx := context.Background()
	store := &Local{Root: t.TempDir()}
	locator, err := store.Put(ctx, PDFKey("doc-1"), strings.NewReader("%PDF-1.7 body"), 13, "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if locator != filepath.Join(store.Root, "doc-1.pdf") {
		t.Fatalf("local locator must be an absolute path under root, got %q", locator)
	}
	info, err := store.Stat(ctx, locator)
	if err != nil || info.Size != 13 {
		t.Fatalf("stat = %+v, %v", info, err)
	}
	head, err := store.ReadHead(ctx, locator, 5)
	if err != nil || string(head) != "%PDF-" {
		t.Fatalf("head = %q, %v", head, err)
	}
	asset, err := store.Put(ctx, AssetPrefix("doc-1")+"abc.png", strings.NewReader("png"), 3, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteDocument(ctx, "doc-1", locator); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{locator, asset} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s should be deleted, err = %v", path, err)
		}
	}
}

func TestLocalRejectsLocatorsOutsideRoot(t *testing.T) {
	store := &Local{Root: t.TempDir()}
	for _, locator := range []string{"/etc/passwd", filepath.Join(store.Root, "..", "escape.pdf"), "s3://bucket/key", "relative.pdf"} {
		if _, err := store.Stat(context.Background(), locator); !errors.Is(err, ErrInvalidLocator) {
			t.Errorf("Stat(%q) err = %v, want ErrInvalidLocator", locator, err)
		}
	}
	if _, err := store.PresignPut(context.Background(), "users/u/uploads/x.pdf", "application/pdf", 10, time.Minute); !errors.Is(err, ErrDirectUploadUnsupported) {
		t.Fatalf("local presign err = %v", err)
	}
}

func TestUserUploadKeyIsServerGeneratedAndScoped(t *testing.T) {
	key, err := UserUploadKey("6f1c1f43-6a58-4c7e-8b38-1b3c55f0a1aa", "0d7a4c52-90b6-4b54-9a8d-2f1b6a3c4d5e")
	if err != nil || key != "users/6f1c1f43-6a58-4c7e-8b38-1b3c55f0a1aa/uploads/0d7a4c52-90b6-4b54-9a8d-2f1b6a3c4d5e.pdf" {
		t.Fatalf("key = %q, %v", key, err)
	}
	for _, pair := range [][2]string{{"../x", "0d7a4c52-90b6-4b54-9a8d-2f1b6a3c4d5e"}, {"6f1c1f43-6a58-4c7e-8b38-1b3c55f0a1aa", "x/../../y"}} {
		if _, err := UserUploadKey(pair[0], pair[1]); err == nil {
			t.Errorf("UserUploadKey(%q, %q) must reject non-UUID input", pair[0], pair[1])
		}
	}
}

func s3ForTest(t *testing.T, endpoint string) *S3 {
	t.Helper()
	store, err := NewS3(S3Config{
		Endpoint: endpoint, Bucket: "selar-test", Region: "auto",
		AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key", ForcePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestS3PutStatHeadDeleteAgainstFakeEndpoint(t *testing.T) {
	ctx := context.Background()
	fake, server := storagetest.NewFakeS3WithState(t, "selar-test")
	store := s3ForTest(t, server.URL)
	body := []byte("%PDF-1.4 fabricated")
	locator, err := store.Put(ctx, PDFKey("doc-2"), bytes.NewReader(body), int64(len(body)), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if locator != "s3://selar-test/doc-2.pdf" {
		t.Fatalf("locator = %q", locator)
	}
	info, err := store.Stat(ctx, locator)
	if err != nil || info.Size != int64(len(body)) || info.ContentType != "application/pdf" {
		t.Fatalf("stat = %+v, %v", info, err)
	}
	head, err := store.ReadHead(ctx, locator, 5)
	if err != nil || string(head) != "%PDF-" {
		t.Fatalf("head = %q, %v", head, err)
	}
	if _, err := store.Put(ctx, AssetPrefix("doc-2")+"a.png", strings.NewReader("png"), 3, "image/png"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteDocument(ctx, "doc-2", locator); err != nil {
		t.Fatal(err)
	}
	if fake.Len() != 0 {
		t.Fatalf("objects left after delete: %v", fake.Keys())
	}
	if _, err := store.Stat(ctx, locator); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing object err = %v, want ErrNotFound", err)
	}
}

func TestS3RejectsForeignBucketLocators(t *testing.T) {
	server := storagetest.NewFakeS3(t, "selar-test")
	store := s3ForTest(t, server.URL)
	for _, locator := range []string{"s3://other-bucket/documents/x.pdf", "/tmp/selar_uploads/x.pdf", "s3://selar-test/../x"} {
		if _, err := store.Stat(context.Background(), locator); !errors.Is(err, ErrInvalidLocator) {
			t.Errorf("Stat(%q) err = %v, want ErrInvalidLocator", locator, err)
		}
	}
}

func TestS3PresignPutBindsTypeLengthAndExpiryWithoutLeakingSecret(t *testing.T) {
	server := storagetest.NewFakeS3(t, "selar-test")
	store := s3ForTest(t, server.URL)
	upload, err := store.PresignPut(context.Background(), "users/u/uploads/x.pdf", "application/pdf", 1234, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(upload.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Path != "/selar-test/users/u/uploads/x.pdf" {
		t.Fatalf("path = %q", parsed.Path)
	}
	if query.Get("X-Amz-Expires") != "600" {
		t.Fatalf("expiry = %q", query.Get("X-Amz-Expires"))
	}
	signed := query.Get("X-Amz-SignedHeaders")
	if !strings.Contains(signed, "content-length") || !strings.Contains(signed, "content-type") {
		t.Fatalf("signed headers %q must bind length and type", signed)
	}
	if upload.Headers["Content-Type"] != "application/pdf" || upload.Method != http.MethodPut {
		t.Fatalf("upload instructions = %+v", upload)
	}
	if strings.Contains(upload.URL, "test-secret-key") {
		t.Fatal("presigned URL leaked the secret key")
	}

	get, err := store.PresignGet(context.Background(), "s3://selar-test/documents/x.pdf", 5*time.Minute, "application/pdf")
	if err != nil || !strings.Contains(get, "X-Amz-Expires=300") {
		t.Fatalf("presigned get = %q, %v", get, err)
	}
}
