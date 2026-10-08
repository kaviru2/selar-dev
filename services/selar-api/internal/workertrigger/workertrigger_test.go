package workertrigger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDisabledWithoutSecretOrURL(t *testing.T) {
	for _, values := range []map[string]string{
		{},
		{"WORKER_URL": "http://worker:8000"},
		{"WORKER_TRIGGER_SECRET": "s3cret-value-long-enough"},
	} {
		client := FromEnv(env(values))
		if client.Enabled() {
			t.Fatalf("trigger must be disabled for %v (local mode relies on the polling worker)", values)
		}
		if err := client.Notify(context.Background(), "11111111-1111-1111-1111-111111111111"); err != nil {
			t.Fatalf("disabled notify must be a no-op, got %v", err)
		}
	}
}

func TestNotifySendsSecretHeaderAndJobID(t *testing.T) {
	var got struct {
		path, secret, contentType string
		body                      map[string]string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.secret = r.Header.Get(SecretHeader)
		got.contentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	client := FromEnv(env(map[string]string{"WORKER_URL": server.URL + "/", "WORKER_TRIGGER_SECRET": "s3cret-value-long-enough"}))
	if !client.Enabled() {
		t.Fatal("expected enabled client")
	}
	if err := client.Notify(context.Background(), "11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}
	if got.path != "/jobs/trigger" || got.secret != "s3cret-value-long-enough" || got.contentType != "application/json" {
		t.Fatalf("request = %+v", got)
	}
	if got.body["job_id"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("body = %v", got.body)
	}
}

func TestExplicitTriggerURLOverridesWorkerURL(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/custom" {
			atomic.AddInt32(&hits, 1)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	client := FromEnv(env(map[string]string{
		"WORKER_URL": "http://unused.invalid", "WORKER_TRIGGER_URL": server.URL + "/custom",
		"WORKER_TRIGGER_SECRET": "s3cret-value-long-enough",
	}))
	if err := client.Notify(context.Background(), "11111111-1111-1111-1111-111111111111"); err != nil || hits != 1 {
		t.Fatalf("hits=%d err=%v", hits, err)
	}
}

func TestNotifyIsBoundedAndErrorsHideTheSecret(t *testing.T) {
	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer server.Close()
	defer close(block)
	client := FromEnv(env(map[string]string{
		"WORKER_URL": server.URL, "WORKER_TRIGGER_SECRET": "s3cret-value-long-enough", "WORKER_TRIGGER_TIMEOUT": "150ms",
	}))
	started := time.Now()
	err := client.Notify(context.Background(), "11111111-1111-1111-1111-111111111111")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("notify took %s; it must be bounded by WORKER_TRIGGER_TIMEOUT", elapsed)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error leaked the secret: %v", err)
	}
}

func TestNotifyReportsRejectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	client := FromEnv(env(map[string]string{"WORKER_URL": server.URL, "WORKER_TRIGGER_SECRET": "s3cret-value-long-enough"}))
	if err := client.Notify(context.Background(), "11111111-1111-1111-1111-111111111111"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestNotifyRejectsNonUUIDJobIDs(t *testing.T) {
	client := FromEnv(env(map[string]string{"WORKER_URL": "http://worker.invalid", "WORKER_TRIGGER_SECRET": "s3cret-value-long-enough"}))
	if err := client.Notify(context.Background(), "../../x"); err == nil {
		t.Fatal("expected invalid job id error")
	}
}

func TestAuthorizeAddsHeaderOnlyWhenConfigured(t *testing.T) {
	request, _ := http.NewRequest(http.MethodPost, "http://worker/chat", nil)
	FromEnv(env(map[string]string{"WORKER_TRIGGER_SECRET": "s3cret-value-long-enough"})).Authorize(request)
	if request.Header.Get(SecretHeader) != "s3cret-value-long-enough" {
		t.Fatal("chat requests must carry the worker secret when configured")
	}
	plain, _ := http.NewRequest(http.MethodPost, "http://worker/chat", nil)
	FromEnv(env(nil)).Authorize(plain)
	if plain.Header.Get(SecretHeader) != "" {
		t.Fatal("no header without a secret")
	}
}

// Serverless runtimes (Vercel) freeze the function once the handler returns,
// so the trigger must already be delivered when NotifyBestEffort returns.
func TestNotifyBestEffortDeliversBeforeReturning(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	client := FromEnv(env(map[string]string{"WORKER_URL": server.URL, "WORKER_TRIGGER_SECRET": "s3cret-value-long-enough"}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // a client that already disconnected must not drop the trigger
	client.NotifyBestEffort(ctx, "11111111-1111-1111-1111-111111111111")
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("trigger delivered %d times before return, want 1", got)
	}
}

func TestNotifyBestEffortIsBoundedAndTolerant(t *testing.T) {
	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer server.Close()
	defer close(block)
	client := FromEnv(env(map[string]string{
		"WORKER_URL": server.URL, "WORKER_TRIGGER_SECRET": "s3cret-value-long-enough", "WORKER_TRIGGER_TIMEOUT": "150ms",
	}))
	started := time.Now()
	client.NotifyBestEffort(context.Background(), "11111111-1111-1111-1111-111111111111")
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("best-effort notify took %s; it must be bounded by WORKER_TRIGGER_TIMEOUT", elapsed)
	}
	FromEnv(env(map[string]string{})).NotifyBestEffort(context.Background(), "11111111-1111-1111-1111-111111111111")
}
