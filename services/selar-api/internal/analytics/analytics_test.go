package analytics

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestUnknownEventIsRejected(t *testing.T) {
	if _, err := Sanitize(Event("free_text_dump"), Props{"x": 1}, SourceServer); err == nil {
		t.Fatal("events outside the allow-list must be rejected")
	}
}

func TestSanitizeDropsUnknownAndFreeTextProps(t *testing.T) {
	props, err := Sanitize(ChatQuestionAsked, Props{
		"length":  42,
		"content": "what is backprop?", // free text is never stored
		"email":   "a@example.com",
	}, SourceServer)
	if err != nil {
		t.Fatal(err)
	}
	if len(props) != 1 || props["length"] != int64(42) {
		t.Fatalf("only the allow-listed numeric prop may survive, got %#v", props)
	}
}

func TestSanitizeValidatesEnumsIDsAndRanges(t *testing.T) {
	props, err := Sanitize(DecisionMade, Props{
		"action":      "keep",
		"kind":        "nonsense",
		"response_ms": -5,
		"link_id":     "not-a-uuid",
	}, SourceServer)
	if err != nil {
		t.Fatal(err)
	}
	if props["action"] != "keep" {
		t.Fatalf("valid enum dropped: %#v", props)
	}
	for _, key := range []string{"kind", "response_ms", "link_id"} {
		if _, ok := props[key]; ok {
			t.Fatalf("invalid %s must be dropped: %#v", key, props)
		}
	}
	props, _ = Sanitize(DecisionMade, Props{"link_id": "6f1c1d0e-8d1c-4e7e-9a1b-111111111111", "response_ms": 1500.7}, SourceServer)
	if props["link_id"] != "6f1c1d0e-8d1c-4e7e-9a1b-111111111111" || props["response_ms"] != int64(1501) {
		t.Fatalf("valid id/number dropped or not rounded: %#v", props)
	}
}

func TestClientCannotForgeServerTruthEvents(t *testing.T) {
	if _, err := Sanitize(DecisionMade, Props{"action": "keep"}, SourceClient); err == nil {
		t.Fatal("decision_made is server-side truth and must not be accepted from the browser")
	}
	if _, err := Sanitize(ReaderPageViewed, Props{"page": 3, "dwell_ms": 1200}, SourceClient); err != nil {
		t.Fatalf("UI-only events must be accepted from the browser: %v", err)
	}
}

func TestRouteStripsQueryAndRejectsLongValues(t *testing.T) {
	props, _ := Sanitize(PageViewed, Props{"route": "/reader?doc=secret&q=text"}, SourceClient)
	if props["route"] != "/reader" {
		t.Fatalf("query strings must be stripped from routes, got %#v", props)
	}
	props, _ = Sanitize(PageViewed, Props{"route": "/" + strings.Repeat("a", 200)}, SourceClient)
	if _, ok := props["route"]; ok {
		t.Fatal("over-long routes must be dropped")
	}
	props, _ = Sanitize(PageViewed, Props{"route": "https://evil.example/x"}, SourceClient)
	if _, ok := props["route"]; ok {
		t.Fatal("absolute URLs are not routes")
	}
}

func TestDwellIsCapped(t *testing.T) {
	props, _ := Sanitize(ReaderPageViewed, Props{"page": 2, "dwell_ms": 99 * 60 * 1000}, SourceClient)
	if _, ok := props["dwell_ms"]; ok {
		t.Fatal("dwell above the documented cap must be dropped, not stored")
	}
}

func TestClampClientTime(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	if got := ClampClientTime(now.Add(-time.Hour), now); !got.Equal(now.Add(-time.Hour)) {
		t.Fatalf("recent client time must be kept, got %v", got)
	}
	for _, bad := range []time.Time{{}, now.Add(time.Hour), now.Add(-48 * time.Hour)} {
		if got := ClampClientTime(bad, now); !got.Equal(now) {
			t.Fatalf("implausible client time %v must fall back to server time, got %v", bad, got)
		}
	}
}

func TestEveryEventIsDocumentedWithASource(t *testing.T) {
	for _, spec := range Dictionary() {
		if spec.Description == "" || (spec.Source != SourceServer && spec.Source != SourceClient) {
			t.Fatalf("event %s needs a description and a source", spec.Name)
		}
	}
}

func TestParseAdminEmails(t *testing.T) {
	set := ParseEmailList(" Alice@Example.com, ,bob@example.com ")
	if !set.Contains("alice@example.com") || !set.Contains("BOB@example.com") || set.Contains("eve@example.com") {
		t.Fatalf("unexpected set %#v", set)
	}
	if ParseEmailList("").Contains("") {
		t.Fatal("an empty list must not match the empty email")
	}
}

func TestDocsListEveryEvent(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/ANALYTICS.md")
	if err != nil {
		t.Fatalf("docs/ANALYTICS.md must exist: %v", err)
	}
	doc := string(raw)
	for _, spec := range Dictionary() {
		row := "| `" + string(spec.Name) + "` | " + string(spec.Source) + " |"
		if !strings.Contains(doc, row) {
			t.Errorf("docs/ANALYTICS.md is missing the row for %s (%s)", spec.Name, spec.Source)
		}
	}
	if !strings.Contains(doc, ConsentVersion) {
		t.Error("docs/ANALYTICS.md must name the current consent version")
	}
}
