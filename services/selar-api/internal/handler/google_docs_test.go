package handler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGoogleDocsExportBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name           string
		upstream, want int
		data           string
	}{
		{"expired", 401, 401, ""}, {"forbidden", 403, 404, ""}, {"missing", 404, 404, ""}, {"outage", 503, 502, ""},
		{"not PDF", 200, 400, "<html>not a PDF</html>"}, {"oversized streamed export", 200, 413, "%PDF-" + strings.Repeat("a", 10<<20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := New(nil)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/export") {
					w.WriteHeader(tc.upstream)
					fmt.Fprint(w, tc.data)
					return
				}
				fmt.Fprint(w, `{"name":"Snapshot","mimeType":"application/vnd.google-apps.document"}`)
			}))
			defer server.Close()
			h.SetDriveAPIBase(server.URL)
			rec := postJSON(t, h.ImportDriveFile, "/api/documents/import/drive", driveBody(driveFileID, driveToken), testUser)
			if rec.Code != tc.want || strings.Contains(rec.Body.String(), driveToken) {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestIntegrationGoogleDocsSnapshotWithoutSize(t *testing.T) {
	h, pool, owner := formatStore(t)
	pdf := []byte("%PDF-1.7 synthetic snapshot bytes")
	exported := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+driveToken {
			t.Error("missing bearer")
			w.WriteHeader(401)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/export") {
			exported = true
			if r.URL.Query().Get("mimeType") != "application/pdf" {
				t.Error("not a PDF export")
			}
			w.Write(pdf)
			return
		}
		fmt.Fprint(w, `{"name":"Native notes","mimeType":"application/vnd.google-apps.document"}`)
	}))
	defer server.Close()
	h.SetDriveAPIBase(server.URL)
	rec := postJSON(t, h.ImportDriveFile, "/api/documents/import/drive", driveBody(driveFileID, driveToken), owner)
	if rec.Code != 202 || !exported {
		t.Fatalf("%d exported=%v %s", rec.Code, exported, rec.Body.String())
	}
	var config string
	if err := pool.QueryRow(context.Background(), `SELECT config::text FROM content_sources WHERE user_id=$1`, owner).Scan(&config); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{driveFileID, "application/vnd.google-apps.document", "imported_at", "snapshot", fmt.Sprintf("%x", sha256.Sum256(pdf))} {
		if !strings.Contains(config, required) {
			t.Fatalf("missing %s: %s", required, config)
		}
	}
	if strings.Contains(config, driveToken) {
		t.Fatal("stored token")
	}
}
