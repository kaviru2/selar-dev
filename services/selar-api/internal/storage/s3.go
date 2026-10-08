package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config configures an S3-compatible backend. SecretAccessKey is never
// logged or included in errors.
type S3Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
}

// S3 is an S3-compatible object store.
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 builds a client. The endpoint may include a scheme (https:// default).
func NewS3(config S3Config) (*S3, error) {
	endpoint := config.Endpoint
	secure := true
	if parsed, err := url.Parse(endpoint); err == nil && parsed.Scheme != "" {
		secure = parsed.Scheme == "https"
		endpoint = parsed.Host
	}
	if endpoint == "" {
		return nil, errors.New("S3_ENDPOINT must be a host or URL")
	}
	lookup := minio.BucketLookupAuto
	if config.ForcePathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure:       secure,
		Region:       config.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, errors.New("invalid S3 configuration")
	}
	return &S3{client: client, bucket: config.Bucket}, nil
}

func (s *S3) Backend() string { return "s3" }

func (s *S3) Locator(key string) string { return "s3://" + s.bucket + "/" + key }

func (s *S3) key(locator string) (string, error) {
	prefix := "s3://" + s.bucket + "/"
	if !strings.HasPrefix(locator, prefix) {
		return "", ErrInvalidLocator
	}
	key := strings.TrimPrefix(locator, prefix)
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", ErrInvalidLocator
	}
	return key, nil
}

func mapS3Error(err error) error {
	if err == nil {
		return nil
	}
	response := minio.ToErrorResponse(err)
	if response.StatusCode == http.StatusNotFound || response.Code == "NoSuchKey" || response.Code == "NotFound" {
		return ErrNotFound
	}
	return fmt.Errorf("object storage request failed: %s", response.Code)
}

func (s *S3) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (string, error) {
	if _, err := s.key(s.Locator(key)); err != nil {
		return "", err
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", mapS3Error(err)
	}
	return s.Locator(key), nil
}

func (s *S3) Stat(ctx context.Context, locator string) (ObjectInfo, error) {
	key, err := s.key(locator)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, mapS3Error(err)
	}
	return ObjectInfo{Size: info.Size, ContentType: info.ContentType}, nil
}

func (s *S3) Open(ctx context.Context, locator string) (io.ReadCloser, ObjectInfo, error) {
	info, err := s.Stat(ctx, locator)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	key, _ := s.key(locator)
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, mapS3Error(err)
	}
	return object, info, nil
}

func (s *S3) ReadHead(ctx context.Context, locator string, n int) ([]byte, error) {
	key, err := s.key(locator)
	if err != nil {
		return nil, err
	}
	options := minio.GetObjectOptions{}
	if err := options.SetRange(0, int64(n-1)); err != nil {
		return nil, err
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, options)
	if err != nil {
		return nil, mapS3Error(err)
	}
	defer object.Close()
	head := make([]byte, n)
	read, err := io.ReadFull(object, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, mapS3Error(err)
	}
	return head[:read], nil
}

func (s *S3) Delete(ctx context.Context, locator string) error {
	key, err := s.key(locator)
	if err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		if mapped := mapS3Error(err); !errors.Is(mapped, ErrNotFound) {
			return mapped
		}
	}
	return nil
}

func (s *S3) DeleteDocument(ctx context.Context, documentID, pdfLocator string) error {
	var firstErr error
	keep := func(err error) {
		if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrInvalidLocator) && firstErr == nil {
			firstErr = err
		}
	}
	keep(s.Delete(ctx, s.Locator(PDFKey(documentID))))
	if pdfLocator != "" && pdfLocator != s.Locator(PDFKey(documentID)) {
		keep(s.Delete(ctx, pdfLocator))
	}
	for object := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: AssetPrefix(documentID), Recursive: true}) {
		if object.Err != nil {
			keep(mapS3Error(object.Err))
			break
		}
		keep(s.Delete(ctx, s.Locator(object.Key)))
	}
	return firstErr
}

func (s *S3) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (DirectUpload, error) {
	if _, err := s.key(s.Locator(key)); err != nil {
		return DirectUpload{}, err
	}
	// Content-Type and Content-Length are part of the signature, so the
	// browser cannot upload a different type or a larger body with this URL.
	headers := http.Header{}
	headers.Set("Content-Type", contentType)
	headers.Set("Content-Length", strconv.FormatInt(size, 10))
	signed, err := s.client.PresignHeader(ctx, http.MethodPut, s.bucket, key, ttl, nil, headers)
	if err != nil {
		return DirectUpload{}, errors.New("could not presign upload")
	}
	return DirectUpload{
		Method:    http.MethodPut,
		URL:       signed.String(),
		Headers:   map[string]string{"Content-Type": contentType},
		ExpiresAt: time.Now().Add(ttl).UTC(),
	}, nil
}

func (s *S3) PresignGet(ctx context.Context, locator string, ttl time.Duration, contentType string) (string, error) {
	key, err := s.key(locator)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	if contentType != "" {
		params.Set("response-content-type", contentType)
	}
	params.Set("response-content-disposition", "inline")
	signed, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, params)
	if err != nil {
		return "", errors.New("could not presign download")
	}
	return signed.String(), nil
}
