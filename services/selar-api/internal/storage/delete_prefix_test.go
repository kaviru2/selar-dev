package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/selar-dev/selar-api/internal/storage/storagetest"
)

func TestS3DeletePrefixRemovesOnlyThatPrefix(t *testing.T) {
	fake, server := storagetest.NewFakeS3WithState(t, "selar-test")
	store, err := NewS3(S3Config{Endpoint: server.URL, Bucket: "selar-test", Region: "auto",
		AccessKeyID: "a", SecretAccessKey: "b", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	fake.Seed("users/u1/uploads/a.pdf", []byte("a"), "application/pdf")
	fake.Seed("users/u1/uploads/b.pdf", []byte("b"), "application/pdf")
	fake.Seed("users/u10/uploads/c.pdf", []byte("c"), "application/pdf")
	if err := store.DeletePrefix(context.Background(), UserUploadPrefix("u1")); err != nil {
		t.Fatal(err)
	}
	if fake.Has("users/u1/uploads/a.pdf") || fake.Has("users/u1/uploads/b.pdf") || !fake.Has("users/u10/uploads/c.pdf") {
		t.Fatalf("keys after delete: %v", fake.Keys())
	}
}

func TestLocalDeletePrefixRemovesOnlyThatPrefix(t *testing.T) {
	root := t.TempDir()
	store := &Local{Root: root}
	for _, key := range []string{"users/u1/uploads/a.pdf", "users/u10/uploads/c.pdf"} {
		if _, err := store.Put(context.Background(), key, strings.NewReader("x"), 1, "application/pdf"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeletePrefix(context.Background(), UserUploadPrefix("u1")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "users/u1/uploads/a.pdf")); !os.IsNotExist(err) {
		t.Fatal("u1 upload survived")
	}
	if _, err := os.Stat(filepath.Join(root, "users/u10/uploads/c.pdf")); err != nil {
		t.Fatal("u10 upload was removed")
	}
	if err := store.DeletePrefix(context.Background(), "../outside/"); err == nil {
		t.Fatal("prefix escaping the root must be rejected")
	}
}
