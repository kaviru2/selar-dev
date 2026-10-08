// Package storage abstracts where uploaded PDFs and extracted visual assets
// live. The local backend keeps the historical /tmp/selar_uploads layout for
// Docker Compose and local development. The S3 backend works with any
// S3-compatible store (Cloudflare R2, Supabase Storage, AWS S3, MinIO).
//
// A "locator" is what the database stores in ingestion_jobs.file_path and
// assets.storage_path: an absolute path for local, s3://bucket/key for S3.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrNotFound means the object does not exist.
	ErrNotFound = errors.New("storage object not found")
	// ErrInvalidLocator means a locator does not belong to this store.
	ErrInvalidLocator = errors.New("storage locator does not belong to this store")
	// ErrDirectUploadUnsupported means the backend cannot issue presigned URLs.
	ErrDirectUploadUnsupported = errors.New("direct uploads require STORAGE_BACKEND=s3")
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Size        int64
	ContentType string
}

// DirectUpload is what the browser needs to PUT bytes straight to storage.
type DirectUpload struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// Store is implemented by every backend.
type Store interface {
	Backend() string
	// Put stores body under key and returns the locator to persist.
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (string, error)
	Stat(ctx context.Context, locator string) (ObjectInfo, error)
	Open(ctx context.Context, locator string) (io.ReadCloser, ObjectInfo, error)
	ReadHead(ctx context.Context, locator string, n int) ([]byte, error)
	Delete(ctx context.Context, locator string) error
	// DeleteDocument removes the PDF locator and every derived asset.
	DeleteDocument(ctx context.Context, documentID, pdfLocator string) error
	// DeletePrefix removes every object whose key starts with prefix, for
	// example a user's upload folder when the account is deleted.
	DeletePrefix(ctx context.Context, prefix string) error
	// Locator converts a backend key to the persisted locator form.
	Locator(key string) string
	// PresignPut returns a short-lived PUT bound to type and exact length.
	PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (DirectUpload, error)
	// PresignGet returns a short-lived GET URL, or ErrDirectUploadUnsupported.
	PresignGet(ctx context.Context, locator string, ttl time.Duration, contentType string) (string, error)
}

// PDFKey is the storage key for a document's canonical PDF bytes. It matches
// the historical /tmp/selar_uploads/<id>.pdf layout.
func PDFKey(documentID string) string { return documentID + ".pdf" }

// AssetPrefix is the key prefix for a document's extracted visual assets.
func AssetPrefix(documentID string) string { return documentID + "/assets/" }

// UserUploadKey is the server-generated key for a direct browser upload.
// Both parts must be UUIDs so a client cannot steer the key.
func UserUploadKey(userID, uploadID string) (string, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return "", fmt.Errorf("invalid user id")
	}
	if _, err := uuid.Parse(uploadID); err != nil {
		return "", fmt.Errorf("invalid upload id")
	}
	return "users/" + userID + "/uploads/" + uploadID + ".pdf", nil
}

// UserUploadPrefix is the key prefix all of a user's direct uploads share.
func UserUploadPrefix(userID string) string { return "users/" + userID + "/uploads/" }

// FromEnv builds the configured store. Errors name missing variables but
// never echo values.
func FromEnv(getenv func(string) string) (Store, error) {
	backend := strings.ToLower(strings.TrimSpace(getenv("STORAGE_BACKEND")))
	switch backend {
	case "", "local":
		root := strings.TrimSpace(getenv("LOCAL_STORAGE_DIR"))
		if root == "" {
			root = "/tmp/selar_uploads"
		}
		return &Local{Root: root}, nil
	case "s3":
		config := S3Config{
			Endpoint:        strings.TrimSpace(getenv("S3_ENDPOINT")),
			Bucket:          strings.TrimSpace(getenv("S3_BUCKET")),
			Region:          strings.TrimSpace(getenv("S3_REGION")),
			AccessKeyID:     strings.TrimSpace(getenv("S3_ACCESS_KEY_ID")),
			SecretAccessKey: strings.TrimSpace(getenv("S3_SECRET_ACCESS_KEY")),
			ForcePathStyle:  strings.EqualFold(strings.TrimSpace(getenv("S3_FORCE_PATH_STYLE")), "true"),
		}
		var missing []string
		for name, value := range map[string]string{
			"S3_ENDPOINT": config.Endpoint, "S3_BUCKET": config.Bucket,
			"S3_ACCESS_KEY_ID": config.AccessKeyID, "S3_SECRET_ACCESS_KEY": config.SecretAccessKey,
		} {
			if value == "" {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			sortStrings(missing)
			return nil, fmt.Errorf("STORAGE_BACKEND=s3 requires %s", strings.Join(missing, ", "))
		}
		if config.Region == "" {
			config.Region = "auto"
		}
		return NewS3(config)
	default:
		return nil, fmt.Errorf("STORAGE_BACKEND must be local or s3")
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
