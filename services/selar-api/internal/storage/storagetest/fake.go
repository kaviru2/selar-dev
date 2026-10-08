// Package storagetest provides an in-process fake of the small S3 API surface
// SELAR uses (PUT, HEAD, ranged GET, DELETE, ListObjectsV2). Tests never touch
// the network or a real bucket.
package storagetest

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// FakeS3 is a path-style S3 endpoint for one bucket.
type FakeS3 struct {
	bucket  string
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string
}

// NewFakeS3 starts a fake endpoint and returns it.
func NewFakeS3(t *testing.T, bucket string) *httptest.Server {
	_, server := NewFakeS3WithState(t, bucket)
	return server
}

// NewFakeS3WithState also returns the fake so tests can inspect/seed objects.
func NewFakeS3WithState(t *testing.T, bucket string) (*FakeS3, *httptest.Server) {
	t.Helper()
	fake := &FakeS3{bucket: bucket, objects: map[string][]byte{}, types: map[string]string{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server
}

// Seed stores an object directly (simulates a browser presigned PUT).
func (f *FakeS3) Seed(key string, body []byte, contentType string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = append([]byte(nil), body...)
	f.types[key] = contentType
}

// Len returns the number of stored objects.
func (f *FakeS3) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}

// Has reports whether key exists.
func (f *FakeS3) Has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok
}

// Keys returns stored keys in sorted order.
func (f *FakeS3) Keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.objects))
	for key := range f.objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (f *FakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	bucket, key := parts[0], ""
	if len(parts) == 2 {
		key = parts[1]
	}
	if bucket != f.bucket {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch {
	case r.Method == http.MethodGet && key == "" && r.URL.Query().Get("list-type") == "2":
		type content struct{ Key string }
		result := struct {
			XMLName     xml.Name `xml:"ListBucketResult"`
			Name        string
			Prefix      string
			KeyCount    int
			IsTruncated bool
			Contents    []content
		}{Name: bucket, Prefix: r.URL.Query().Get("prefix")}
		var keys []string
		for existing := range f.objects {
			if strings.HasPrefix(existing, result.Prefix) {
				keys = append(keys, existing)
			}
		}
		sort.Strings(keys)
		for _, existing := range keys {
			result.Contents = append(result.Contents, content{Key: existing})
		}
		result.KeyCount = len(keys)
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(result)
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		if strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-") {
			body = decodeAWSChunked(body)
		}
		f.objects[key] = body
		f.types[key] = r.Header.Get("Content-Type")
		w.Header().Set("ETag", `"etag"`)
	case r.Method == http.MethodHead || r.Method == http.MethodGet:
		body, ok := f.objects[key]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			}
			return
		}
		w.Header().Set("Content-Type", f.types[key])
		w.Header().Set("ETag", `"etag"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if rangeHeader := r.Header.Get("Range"); rangeHeader != "" && r.Method == http.MethodGet {
			start, end := parseRange(rangeHeader)
			if end >= len(body) {
				end = len(body) - 1
			}
			w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(end)+"/"+strconv.Itoa(len(body)))
			w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[start : end+1])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	case r.Method == http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func parseRange(header string) (int, int) {
	pieces := strings.SplitN(strings.TrimPrefix(header, "bytes="), "-", 2)
	start, _ := strconv.Atoi(pieces[0])
	end, _ := strconv.Atoi(pieces[1])
	return start, end
}

// decodeAWSChunked strips SigV4 streaming framing
// ("<hex>;chunk-signature=...\r\n<data>\r\n", ending with a 0-size chunk).
func decodeAWSChunked(raw []byte) []byte {
	var out []byte
	for len(raw) > 0 {
		lineEnd := bytes.Index(raw, []byte("\r\n"))
		if lineEnd < 0 {
			break
		}
		header := string(raw[:lineEnd])
		if semi := strings.IndexByte(header, ';'); semi >= 0 {
			header = header[:semi]
		}
		size, err := strconv.ParseInt(header, 16, 64)
		if err != nil || size == 0 {
			break
		}
		start := lineEnd + 2
		out = append(out, raw[start:start+int(size)]...)
		raw = raw[start+int(size)+2:]
	}
	return out
}
