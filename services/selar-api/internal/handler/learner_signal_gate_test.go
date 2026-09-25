package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/middleware"
)

func TestUnapprovedAssessmentSignalsCannotWriteOutcomes(t *testing.T) {
	auth := middleware.NewAuth("test-only-secret")
	token, err := auth.GenerateToken("00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	// A nil store makes any attempted persistence a test failure. This
	// endpoint has no validated quiz attempt or assessment workflow yet.
	h := New(nil)
	router := chi.NewRouter()
	router.With(auth.Verify).Post("/api/learner-signals", h.RecordLearnerSignal)

	for _, signal := range []string{"quiz_success", "quiz_failure"} {
		t.Run(signal, func(t *testing.T) {
			body := `{"concept_id":"00000000-0000-0000-0000-000000000002","signal":"` + signal + `","idempotency_id":"invented-attempt"}`
			request := httptest.NewRequest(http.MethodPost, "/api/learner-signals", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusConflict {
				t.Fatalf("unverified %s recorded or misclassified: status=%d body=%s", signal, response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "assessment") {
				t.Fatalf("missing assessment-unavailable explanation: %s", response.Body.String())
			}
		})
	}
}

func TestAssessmentSignalRouteStillRequiresAuthentication(t *testing.T) {
	auth := middleware.NewAuth("test-only-secret")
	router := chi.NewRouter()
	router.With(auth.Verify).Post("/api/learner-signals", New(nil).RecordLearnerSignal)
	request := httptest.NewRequest(http.MethodPost, "/api/learner-signals", strings.NewReader(`{"signal":"quiz_success"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated signal returned %d, want 401", response.Code)
	}
}
