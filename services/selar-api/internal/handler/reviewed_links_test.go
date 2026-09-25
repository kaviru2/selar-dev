package handler

import (
	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReviewAPIRequiresAuthenticatedRevisionToken(t *testing.T) {
	auth := middleware.NewAuth("test-only-secret")
	h := New(nil)
	r := chi.NewRouter()
	r.With(auth.Verify).Post("/api/mental-model-links/{id}/respond", h.RespondToMentalModelLink)
	send := func(body, token string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/mental-model-links/00000000-0000-0000-0000-000000000000/respond", strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if code := send(`{"action":"confirmed","revision":0}`, ""); code != 401 {
		t.Fatalf("unauthenticated: %d", code)
	}
	token, err := auth.GenerateToken("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if code := send(`{"action":"confirmed"}`, token); code != 400 {
		t.Fatalf("missing revision: %d", code)
	}
	if code := send(`{"action":"claim_extension","revision":0}`, token); code != 400 {
		t.Fatalf("unsupported claim: %d", code)
	}
}
