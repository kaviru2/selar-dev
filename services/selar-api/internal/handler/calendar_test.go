package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/allowlist"
	"github.com/selar-dev/selar-api/internal/linktoken"
	"github.com/selar-dev/selar-api/internal/quiz"
)

func calendarFeedRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/calendar/{token}", h.ServeCalendarFeed)
	return r
}

func TestCalendarFeedRejectsForgedTokensWithoutStore(t *testing.T) {
	h := New(nil)
	h.SetCalendarConfig(CalendarConfig{Gate: allowlist.FromEnv("true", "*"), Signer: linktoken.New("k")})
	for _, path := range []string{"/calendar/abc.ics", "/calendar/abc", "/calendar/" + strings.Repeat("A", 48) + ".ics"} {
		rec := httptest.NewRecorder()
		calendarFeedRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	// An unsubscribe token is not a calendar token.
	tok, _ := linktoken.New("k").Issue(linktoken.Unsubscribe, "6f1c1f0e-4b7a-4c39-9a51-0b7d2b1f8c11", 0)
	rec := httptest.NewRecorder()
	calendarFeedRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/calendar/"+tok+".ics", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-purpose token: %d", rec.Code)
	}
}

func TestCalendarUpdateValidatesBeforeStore(t *testing.T) {
	h := New(nil)
	for _, body := range []string{`{}`, `nope`, `{"enabled":"yes"}`} {
		if rec := sendJSON(t, h.UpdateCalendarSettings, http.MethodPut, "/", body, "00000000-0000-0000-0000-000000000001"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
}

func fetchFeed(t *testing.T, h *Handler, feedURL string) (int, string) {
	t.Helper()
	i := strings.Index(feedURL, "/calendar/")
	rec := httptest.NewRecorder()
	calendarFeedRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, feedURL[i:], nil))
	return rec.Code, rec.Body.String()
}

func TestIntegrationCalendarFeedLifecycle(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	colombo, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, colombo)
	open := time.Date(2026, 10, 12, 9, 0, 0, 0, colombo)
	close := time.Date(2026, 10, 14, 21, 0, 0, 0, colombo)
	quizID, err := f.st.CreateQuiz(ctx, f.id, quiz.Draft{
		Title: "SECRET-TITLE treatment arm quiz", Kind: quiz.KindInitial,
		Settings:  quiz.Settings{OpenAt: &open, CloseAt: &close, MaxAttempts: 1, Feedback: quiz.FeedbackNever},
		Questions: []quiz.Question{{Type: quiz.TypeTrueFalse, Prompt: "p", Points: 1, Options: []quiz.Option{{Text: "True", Correct: true}, {Text: "False"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM quizzes WHERE id = $1`, quizID) })
	if err := f.st.SetQuizStatus(ctx, quizID, quiz.StatusPublished); err != nil {
		t.Fatal(err)
	}

	// 1. Server switch off: not available, cannot be enabled.
	f.h.SetCalendarConfig(CalendarConfig{Gate: allowlist.FromEnv("false", "*"), Signer: linktoken.New("test-only-secret"),
		PublicAPIURL: "https://api.test", ConsoleURL: "https://console.test", Now: func() time.Time { return now }})
	rec := sendJSON(t, f.h.GetCalendarSettings, http.MethodGet, "/", "", f.id)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"available":false`) {
		t.Fatalf("off: %d %s", rec.Code, rec.Body.String())
	}
	if rec := sendJSON(t, f.h.UpdateCalendarSettings, http.MethodPut, "/", `{"enabled":true}`, f.id); rec.Code != http.StatusForbidden {
		t.Fatalf("enable while off: %d", rec.Code)
	}
	// Flag on but account not allowlisted: still refused.
	f.h.SetCalendarConfig(CalendarConfig{Gate: allowlist.FromEnv("true", "someone-else@example.com"), Signer: linktoken.New("test-only-secret"),
		PublicAPIURL: "https://api.test", ConsoleURL: "https://console.test", Now: func() time.Time { return now }})
	if rec := sendJSON(t, f.h.UpdateCalendarSettings, http.MethodPut, "/", `{"enabled":true}`, f.id); rec.Code != http.StatusForbidden {
		t.Fatalf("enable when not allowlisted: %d", rec.Code)
	}

	// 2. Allowlisted: enable and read the feed.
	f.h.SetCalendarConfig(CalendarConfig{Gate: allowlist.FromEnv("true", f.email), Signer: linktoken.New("test-only-secret"),
		PublicAPIURL: "https://api.test", ConsoleURL: "https://console.test", Now: func() time.Time { return now }})
	rec = sendJSON(t, f.h.UpdateCalendarSettings, http.MethodPut, "/", `{"enabled":true}`, f.id)
	var view calendarSettingsView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != 200 || !view.Enabled || !strings.HasPrefix(view.FeedURL, "https://api.test/calendar/") {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body.String())
	}
	// The test database may hold other tests' quizzes; look for ours.
	found := false
	for _, e := range view.Events {
		found = found || strings.Contains(e.GoogleURL, "dates=20261012T033000Z%2F20261014T153000Z")
	}
	if !found {
		t.Fatalf("events = %+v", view.Events)
	}
	code, ics := fetchFeed(t, f.h, view.FeedURL)
	if code != 200 || !strings.Contains(ics, "DTSTART:20261012T033000Z\r\n") || !strings.Contains(ics, "DTEND:20261014T153000Z\r\n") {
		t.Fatalf("feed %d:\n%s", code, ics)
	}
	for _, leak := range []string{"SECRET-TITLE", "treatment", "control", f.id, f.email} {
		if strings.Contains(ics, leak) {
			t.Fatalf("feed leaks %q", leak)
		}
	}

	// 3. The admin moves the window: the same URL reflects it.
	newClose := close.Add(24 * time.Hour)
	rec2, err := f.st.GetQuiz(ctx, quizID)
	if err != nil {
		t.Fatal(err)
	}
	d := quiz.Draft{Title: rec2.Title, Kind: rec2.Kind, Settings: rec2.Settings, Questions: rec2.Questions}
	d.Settings.CloseAt = &newClose
	if err := f.st.UpdateQuiz(ctx, quizID, d); err != nil {
		t.Fatal(err)
	}
	if _, ics = fetchFeed(t, f.h, view.FeedURL); !strings.Contains(ics, "DTEND:20261015T153000Z\r\n") {
		t.Fatalf("window change not reflected:\n%s", ics)
	}

	// 4. Reset: the old URL returns an empty calendar, the new one works.
	old := view.FeedURL
	rec = sendJSON(t, f.h.ResetCalendarFeed, http.MethodPost, "/", "", f.id)
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if view.FeedURL == old || !view.Enabled {
		t.Fatalf("reset: %s", rec.Body.String())
	}
	if code, ics = fetchFeed(t, f.h, old); code != 200 || strings.Contains(ics, "BEGIN:VEVENT") {
		t.Fatalf("revoked URL still lists events (%d)", code)
	}
	if _, ics = fetchFeed(t, f.h, view.FeedURL); !strings.Contains(ics, "BEGIN:VEVENT") {
		t.Fatal("new URL has no events")
	}

	// 5. The research team switches the feature off: feed empties.
	f.h.SetCalendarConfig(CalendarConfig{Gate: allowlist.FromEnv("false", f.email), Signer: linktoken.New("test-only-secret"), Now: func() time.Time { return now }})
	if _, ics = fetchFeed(t, f.h, view.FeedURL); strings.Contains(ics, "BEGIN:VEVENT") {
		t.Fatal("feed must be empty when the server switch is off")
	}

	// 6. Opting out revokes the URL even if the switch comes back on.
	f.h.SetCalendarConfig(CalendarConfig{Gate: allowlist.FromEnv("true", f.email), Signer: linktoken.New("test-only-secret"), Now: func() time.Time { return now }})
	last := view.FeedURL
	if rec := sendJSON(t, f.h.UpdateCalendarSettings, http.MethodPut, "/", `{"enabled":false}`, f.id); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	sendJSON(t, f.h.UpdateCalendarSettings, http.MethodPut, "/", `{"enabled":true}`, f.id)
	if _, ics = fetchFeed(t, f.h, last); strings.Contains(ics, "BEGIN:VEVENT") {
		t.Fatal("URL from before opting out must stay revoked")
	}

	// 7. After the quiz is submitted it leaves the calendar.
	if _, err := f.pool.Exec(ctx, `INSERT INTO quiz_attempts(quiz_id,user_id,attempt_number,quiz_version,question_order,started_at,submitted_at,submit_reason)
		VALUES ($1,$2,1,1,'{}',$3,$3,'learner')`, quizID, f.id, open.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ws, err := f.st.CalendarWindows(ctx, f.id, open.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ws {
		if w.QuizID == quizID {
			t.Fatalf("completed quiz must leave the calendar: %+v", w)
		}
	}
}
