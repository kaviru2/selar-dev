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

func TestRegisterRejectsInvalidCredentialsBeforePersistence(t *testing.T) {
	h := handler.New(nil)

	for _, body := range []string{
		`{}`,
		`{"email":"a@b.com"}`,
		`{"email":"   ","password":"eightchars"}`,
		`{"email":"a@b.com","password":"short"}`,
		`not json`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
		w := httptest.NewRecorder()
		h.Register(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Register invalid request %s: got status %d, want %d", body, w.Code, http.StatusBadRequest)
		}
	}
}

func TestStartSessionRequiresDocumentID(t *testing.T) {
	h := handler.New(nil)
	req := httptest.NewRequest(http.MethodPost, "/sessions/start", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	h.StartSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("StartSession missing document ID: got status %d, want %d", w.Code, http.StatusBadRequest)
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
