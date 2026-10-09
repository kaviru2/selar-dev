package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/selar-dev/selar-api/internal/model"
)

func TestChatWorkerReceivesSelectedAssertionAndAuthenticatedOwner(t *testing.T) {
	selection := &model.ChatAssertionSelection{AssertingDocumentID: "00000000-0000-0000-0000-000000000001", AssertionID: "00000000-0000-0000-0000-000000000002", Revision: 2}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			UserID    string                        `json:"user_id"`
			ThreadID  string                        `json:"thread_id"`
			Question  string                        `json:"question"`
			Selection *model.ChatAssertionSelection `json:"assertion_selection"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.UserID != "authenticated-owner" || payload.ThreadID != "owned-thread" || payload.Question != "exact display request" || payload.Selection == nil || *payload.Selection != *selection {
			t.Errorf("unexpected worker request: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer":"saved record","model_version":"deterministic-saved-assertion-v1","citations":[]}`))
	}))
	defer server.Close()
	t.Setenv("WORKER_URL", server.URL)
	answer, err := requestChatAnswer(nil, "authenticated-owner", "owned-thread", "exact display request", nil, selection)
	if err != nil {
		t.Fatal(err)
	}
	if answer.ModelVersion != "deterministic-saved-assertion-v1" {
		t.Fatalf("lost display marker: %+v", answer)
	}
}

func TestChatRejectsMalformedSelectionBeforePersistence(t *testing.T) {
	for _, body := range []string{
		`{"content":"display","assertion_selection":{"asserting_document_id":"bad","assertion_id":"00000000-0000-0000-0000-000000000002","revision":2}}`,
		`{"content":"display","assertion_selection":{"asserting_document_id":"00000000-0000-0000-0000-000000000001","assertion_id":"bad","revision":2}}`,
		`{"content":"display","assertion_selection":{"asserting_document_id":"00000000-0000-0000-0000-000000000001","assertion_id":"00000000-0000-0000-0000-000000000002","revision":0}}`,
	} {
		recorder := httptest.NewRecorder()
		(&Handler{}).CreateChatMessage(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status %d for %s", recorder.Code, body)
		}
	}
}
