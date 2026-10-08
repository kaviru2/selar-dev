package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Local stores objects under Root. Locators are absolute paths, which keeps
// rows written before this abstraction (and the Docker shared volume) valid.
type Local struct {
	Root string
}

func (l *Local) Backend() string { return "local" }

func (l *Local) Locator(key string) string { return filepath.Join(l.Root, filepath.FromSlash(key)) }

func (l *Local) pathFor(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, "..") {
		return "", ErrInvalidLocator
	}
	return filepath.Join(l.Root, clean), nil
}

func (l *Local) resolve(locator string) (string, error) {
	if !filepath.IsAbs(locator) {
		return "", ErrInvalidLocator
	}
	root, err := filepath.Abs(l.Root)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(locator)
	if clean != root && !strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return "", ErrInvalidLocator
	}
	return clean, nil
}

func (l *Local) Put(_ context.Context, key string, body io.Reader, _ int64, _ string) (string, error) {
	path, err := l.pathFor(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, body); err != nil {
		file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (l *Local) Stat(_ context.Context, locator string) (ObjectInfo, error) {
	path, err := l.resolve(locator)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ObjectInfo{}, ErrNotFound
	}
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{Size: info.Size()}, nil
}

func (l *Local) Open(_ context.Context, locator string) (io.ReadCloser, ObjectInfo, error) {
	path, err := l.resolve(locator)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ObjectInfo{}, ErrNotFound
	}
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, ObjectInfo{}, err
	}
	return file, ObjectInfo{Size: info.Size()}, nil
}

func (l *Local) ReadHead(ctx context.Context, locator string, n int) ([]byte, error) {
	reader, _, err := l.Open(ctx, locator)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	head := make([]byte, n)
	read, err := io.ReadFull(reader, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return head[:read], nil
}

func (l *Local) Delete(_ context.Context, locator string) error {
	path, err := l.resolve(locator)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (l *Local) DeleteDocument(ctx context.Context, documentID, pdfLocator string) error {
	var firstErr error
	locators := []string{l.Locator(PDFKey(documentID))}
	if pdfLocator != "" && pdfLocator != locators[0] {
		locators = append(locators, pdfLocator)
	}
	for _, locator := range locators {
		if err := l.Delete(ctx, locator); err != nil && !errors.Is(err, ErrInvalidLocator) && firstErr == nil {
			firstErr = err
		}
	}
	if err := os.RemoveAll(filepath.Join(l.Root, documentID)); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (l *Local) DeletePrefix(_ context.Context, prefix string) error {
	if prefix == "" || !strings.HasSuffix(prefix, "/") {
		return ErrInvalidLocator
	}
	dir, err := l.pathFor(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (l *Local) PresignPut(context.Context, string, string, int64, time.Duration) (DirectUpload, error) {
	return DirectUpload{}, ErrDirectUploadUnsupported
}

func (l *Local) PresignGet(context.Context, string, time.Duration, string) (string, error) {
	return "", ErrDirectUploadUnsupported
}
