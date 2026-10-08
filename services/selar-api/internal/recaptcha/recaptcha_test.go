package recaptcha

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

var fullEnv = map[string]string{
	"RECAPTCHA_PROJECT_ID": "proj-1",
	"RECAPTCHA_SITE_KEY":   "site-key",
	"RECAPTCHA_API_KEY":    "api-key-value",
}

func TestDisabledUnlessFullyConfigured(t *testing.T) {
	for _, missing := range []string{"RECAPTCHA_PROJECT_ID", "RECAPTCHA_SITE_KEY", "RECAPTCHA_API_KEY"} {
		values := map[string]string{}
		for k, v := range fullEnv {
			if k != missing {
				values[k] = v
			}
		}
		c := FromEnv(env(values))
		if c.Enabled() {
			t.Fatalf("enabled without %s", missing)
		}
		if err := c.Check(context.Background(), "", "register"); err != nil {
			t.Fatalf("disabled client must accept, got %v", err)
		}
	}
	if !FromEnv(env(fullEnv)).Enabled() {
		t.Fatal("expected enabled with all variables set")
	}
}

func TestMinScoreOverride(t *testing.T) {
	values := map[string]string{"RECAPTCHA_MIN_SCORE": "0.7"}
	if got := FromEnv(env(values)).MinScore; got != 0.7 {
		t.Fatalf("MinScore = %v", got)
	}
	for _, bad := range []string{"abc", "-1", "2"} {
		if got := FromEnv(env(map[string]string{"RECAPTCHA_MIN_SCORE": bad})).MinScore; got != DefaultMinScore {
			t.Fatalf("bad %q gave %v", bad, got)
		}
	}
}

func fakeGoogle(t *testing.T, status int, resp string) (*Client, *http.Request) {
	t.Helper()
	var seen http.Request
	var seenBody assessmentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = *r
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		if seenBody.Event.SiteKey != "site-key" || seenBody.Event.ExpectedAction != "register" || seenBody.Event.Token == "" {
			t.Errorf("unexpected assessment body %+v", seenBody)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	c := FromEnv(env(fullEnv))
	c.Endpoint = srv.URL
	return c, &seen
}

func TestCheckOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		resp   string
		want   error
	}{
		{"human", 200, `{"tokenProperties":{"valid":true,"action":"register"},"riskAnalysis":{"score":0.9}}`, nil},
		{"at threshold", 200, `{"tokenProperties":{"valid":true,"action":"register"},"riskAnalysis":{"score":0.3}}`, nil},
		{"bot score", 200, `{"tokenProperties":{"valid":true,"action":"register"},"riskAnalysis":{"score":0.1}}`, ErrRejected},
		{"invalid token", 200, `{"tokenProperties":{"valid":false,"invalidReason":"MALFORMED"}}`, ErrRejected},
		{"expired token", 200, `{"tokenProperties":{"valid":false,"invalidReason":"EXPIRED"},"riskAnalysis":{"score":0.9}}`, ErrRejected},
		{"wrong action", 200, `{"tokenProperties":{"valid":true,"action":"login"},"riskAnalysis":{"score":0.9}}`, ErrRejected},
		{"bad api key", 403, `{"error":{"code":403}}`, ErrRejected},
		{"bad request", 400, `{"error":{"code":400}}`, ErrRejected},
		{"google 500", 500, `oops`, ErrUnavailable},
		{"google 503", 503, ``, ErrUnavailable},
		{"rate limited", 429, ``, ErrUnavailable},
		{"garbled 200", 200, `not json`, ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, seen := fakeGoogle(t, tc.status, tc.resp)
			err := c.Check(context.Background(), "tok", "register")
			if tc.want == nil && err != nil {
				t.Fatalf("got %v, want nil", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if seen.URL.Path != "/projects/proj-1/assessments" {
				t.Fatalf("path %q", seen.URL.Path)
			}
			if seen.URL.RawQuery != "" || seen.Header.Get("X-Goog-Api-Key") != "api-key-value" {
				t.Fatal("API key must travel in the X-Goog-Api-Key header, never the URL")
			}
			if err != nil && strings.Contains(err.Error(), "api-key-value") {
				t.Fatal("error leaks the API key")
			}
		})
	}
}

func TestMissingTokenNeverCallsGoogle(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	c := FromEnv(env(fullEnv))
	c.Endpoint = srv.URL
	if err := c.Check(context.Background(), "  ", "register"); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("got %v", err)
	}
	if called {
		t.Fatal("Google must not be called without a token")
	}
}

func TestUnreachableAndTimeoutAreUnavailable(t *testing.T) {
	c := FromEnv(env(fullEnv))
	c.Endpoint = "http://127.0.0.1:1" // nothing listens here
	if err := c.Check(context.Background(), "tok", "register"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unreachable: got %v", err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { time.Sleep(300 * time.Millisecond) }))
	defer slow.Close()
	c.Endpoint = slow.URL
	c.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
	if err := c.Check(context.Background(), "tok", "register"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("timeout: got %v", err)
	}
}
