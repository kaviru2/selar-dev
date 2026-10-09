package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPracticeRPCOverridesOwnerAndReturnsProviderUnavailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data map[string]any
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			t.Fatal(err)
		}
		if data["user_id"] != "owner" {
			t.Errorf("owner not bound: %v", data)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"unavailable"}`))
	}))
	defer upstream.Close()
	t.Setenv("WORKER_URL", upstream.URL)
	h := New(nil)
	req := authed(httptest.NewRequest("POST", "/api/practice", strings.NewReader(`{"action":"generate","user_id":"borrower"}`)), "owner")
	rec := httptest.NewRecorder()
	h.Practice(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "unavailable") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
