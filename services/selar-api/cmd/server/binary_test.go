package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCompiledBinaryServesHealthWithOnlyEnvironment builds the real server and
// starts it the way Vercel's Go preset does: a dynamic PORT, env-only config,
// no .env files and no migrations at startup.
func TestCompiledBinaryServesHealthWithOnlyEnvironment(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "selar-api")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := exec.CommandContext(ctx, binary)
	server.Dir = dir // no .env files nearby
	server.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		fmt.Sprintf("PORT=%d", port),
		"APP_ENV=production",
		"DATABASE_URL=" + databaseURL,
		"JWT_SECRET=compiled-binary-test-secret-not-for-production",
		"CORS_ORIGIN=https://selar-console.example.test",
	}
	var logs strings.Builder
	server.Stdout, server.Stderr = &logs, &logs
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = server.Wait()
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"status":"ok"`) {
				t.Fatalf("healthz = %d %s", response.StatusCode, body)
			}
			if strings.Contains(logs.String(), "selar_test@") || strings.Contains(logs.String(), "compiled-binary-test-secret") {
				t.Fatalf("server logs leaked a secret:\n%s", logs.String())
			}
			if strings.Contains(logs.String(), "applied migration") {
				t.Fatalf("server must not run migrations on startup:\n%s", logs.String())
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server never became healthy on PORT=%d; logs:\n%s", port, logs.String())
}
