package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/selar-dev/selar-api/internal/handler"
)

func TestHealthEndpoint(t *testing.T) {
	h := handler.New(nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	h.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Health: got status %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("Health: body should contain status ok, got %s", body)
	}
	if !strings.Contains(body, `"service":"selar-api"`) {
		t.Errorf("Health: body should contain service name, got %s", body)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Health: Content-Type got %q, want application/json", ct)
	}
}

func TestRegisterMissingFields(t *testing.T) {
	h := handler.New(nil)

	// Empty body
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Register empty: got status %d, want %d", w.Code, http.StatusBadRequest)
	}

	// Missing password
	req = httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"email":"a@b.com"}`))
	w = httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Register no password: got status %d, want %d", w.Code, http.StatusBadRequest)
	}

	// Invalid JSON
	req = httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`not json`))
	w = httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Register invalid JSON: got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLoginInvalidJSON(t *testing.T) {
	h := handler.New(nil)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`broken`))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Login invalid JSON: got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}
