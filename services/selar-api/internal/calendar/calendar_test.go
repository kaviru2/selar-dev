package calendar

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/selar-dev/selar-api/internal/quiz"
)

func ts(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestColomboWindowIsWrittenAsExactUTCInstant(t *testing.T) {
	colombo, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// An admin sets a window of 09:00–17:30 Colombo time on 12 Oct 2026.
	open := time.Date(2026, 10, 12, 9, 0, 0, 0, colombo)
	close := time.Date(2026, 10, 12, 17, 30, 0, 0, colombo)
	a := quiz.Availability{State: quiz.StateUpcoming, OpensAt: &open, ClosesAt: &close}
	w, ok := FromAvailability("q1", quiz.KindInitial, a, open)
	if !ok {
		t.Fatal("window expected")
	}
	ics := Feed([]Event{ToEvent(w, "u1", "https://selar.example")}, open)
	// 09:00 +05:30 is 03:30Z; 17:30 +05:30 is 12:00Z.
	if !strings.Contains(ics, "DTSTART:20261012T033000Z\r\n") || !strings.Contains(ics, "DTEND:20261012T120000Z\r\n") {
		t.Fatalf("times not exact UTC:\n%s", ics)
	}
	if strings.Contains(ics, "TZID") {
		t.Fatal("no TZID/floating times: UTC only")
	}
	// Round-trip: the UTC instant shown in Colombo is the admin's wall time.
	start, _ := time.Parse("20060102T150405Z", "20261012T033000Z")
	if got := start.In(colombo).Format("15:04"); got != "09:00" {
		t.Fatalf("colombo display = %s", got)
	}
}

func TestFeedMatchesServerEnforcedRelativeWindow(t *testing.T) {
	// Relative window: opens 7 days after first reading, open for 2 days,
	// clipped by an absolute close. Evaluate is what the quiz API enforces.
	first := ts("2026-10-01T04:15:00Z")
	s := quiz.Settings{After: &quiz.RelativeWindow{Anchor: quiz.AnchorFirstReading, Days: 7, WindowDays: 2}, CloseAt: ts("2026-10-09T00:00:00Z")}
	q := quiz.Quiz{ID: "q", Status: quiz.StatusPublished, Kind: quiz.KindFollowUp, Settings: s}
	now := ts("2026-10-02T00:00:00Z")
	a := quiz.Evaluate(q, quiz.Learner{FirstReadingAt: first}, quiz.Attempts{}, *now)
	w, ok := FromAvailability(q.ID, q.Kind, a, *now)
	if !ok {
		t.Fatalf("expected an event for %+v", a)
	}
	e := ToEvent(w, "u", "https://c")
	if !e.Start.Equal(*ts("2026-10-08T04:15:00Z")) || !e.End.Equal(*ts("2026-10-09T00:00:00Z")) {
		t.Fatalf("event %v–%v does not match the enforced window %v–%v", e.Start, e.End, a.OpensAt, a.ClosesAt)
	}
	// Before the anchor exists there is no window, so no event.
	a = quiz.Evaluate(q, quiz.Learner{}, quiz.Attempts{}, *now)
	if _, ok := FromAvailability(q.ID, q.Kind, a, *now); ok {
		t.Fatal("no event before the relative anchor happens")
	}
}

func TestOnlyActionableStatesProduceEvents(t *testing.T) {
	o, c := ts("2026-10-10T00:00:00Z"), ts("2026-10-11T00:00:00Z")
	for state, want := range map[quiz.State]bool{
		quiz.StateUpcoming: true, quiz.StateAvailable: true, quiz.StateInProgress: true,
		quiz.StateHidden: false, quiz.StateCompleted: false, quiz.StateMissed: false,
	} {
		_, ok := FromAvailability("q", quiz.KindPractice, quiz.Availability{State: state, OpensAt: o, ClosesAt: c}, *o)
		if ok != want {
			t.Errorf("%s: got %v", state, ok)
		}
	}
	if _, ok := FromAvailability("q", quiz.KindPractice, quiz.Availability{State: quiz.StateAvailable}, *o); ok {
		t.Error("always-open quiz without times has nothing to put in a calendar")
	}
}

func TestOpenEndedAndCloseOnlyMarkers(t *testing.T) {
	o, c := ts("2026-10-10T00:00:00Z"), ts("2026-10-11T00:00:00Z")
	e := ToEvent(Window{QuizID: "a", Kind: quiz.KindInitial, Start: o}, "u", "https://c")
	if e.Summary != "SELAR: Initial test opens" || e.End.Sub(e.Start) != 30*time.Minute {
		t.Fatalf("%+v", e)
	}
	e = ToEvent(Window{QuizID: "b", Kind: quiz.KindFollowUp, End: c}, "u", "https://c")
	if e.Summary != "SELAR: Follow-up test closes" || !e.End.Equal(*c) {
		t.Fatalf("%+v", e)
	}
}

func TestFeedIsValidICS(t *testing.T) {
	o, c := ts("2026-10-10T00:00:00Z"), ts("2026-10-17T00:00:00Z")
	events := []Event{
		ToEvent(Window{QuizID: "b", Kind: quiz.KindPractice, Start: c, End: ts("2026-10-18T00:00:00Z"), Updated: *o}, "user-1", "https://selar.example/"),
		ToEvent(Window{QuizID: "a", Kind: quiz.KindInitial, Start: o, End: c, Updated: *o}, "user-1", "https://selar.example/"),
	}
	ics := Feed(events, *o)
	if !strings.HasPrefix(ics, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:") || !strings.HasSuffix(ics, "END:VCALENDAR\r\n") {
		t.Fatal("envelope")
	}
	lines := strings.Split(strings.TrimSuffix(ics, "\r\n"), "\r\n")
	begins, ends := 0, 0
	for _, l := range lines {
		if strings.Contains(l, "\n") || strings.Contains(l, "\r") {
			t.Fatalf("bare newline in %q", l)
		}
		if len(l) > 75 {
			t.Fatalf("line longer than 75 octets: %q", l)
		}
		if l == "BEGIN:VEVENT" {
			begins++
		}
		if l == "END:VEVENT" {
			ends++
		}
	}
	if begins != 2 || ends != 2 {
		t.Fatalf("events %d/%d", begins, ends)
	}
	// Sorted by start: initial (a) first.
	if strings.Index(ics, "Initial test") > strings.Index(ics, "Practice quiz") {
		t.Fatal("events not sorted by start")
	}
	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	for _, want := range []string{"UID:" + EventUID("user-1", "a"), "TRANSP:TRANSPARENT", `Open SELAR: https://selar.example/quizzes\n`, "DTSTAMP:20261010T000000Z"} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestEscapingAndFoldingKeepUTF8Intact(t *testing.T) {
	if got := escape(`a;b,c\d` + "\n" + "e"); got != `a\;b\,c\\d\ne` {
		t.Fatalf("escape = %q", got)
	}
	long := "DESCRIPTION:" + strings.Repeat("සිංහල ", 30)
	folded := fold(long)
	for _, l := range strings.Split(strings.TrimSuffix(folded, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Fatalf("line longer than 75 octets: %q", l)
		}
		if strings.ToValidUTF8(l, "?") != l {
			t.Fatalf("split a UTF-8 sequence: %q", l)
		}
	}
	if strings.ReplaceAll(strings.TrimSuffix(folded, "\r\n"), "\r\n ", "") != long {
		t.Fatal("unfold must restore the line")
	}
}

func TestNoStudyArmOrQuizContentLeaks(t *testing.T) {
	// Windows carry no title, cohort or group; the rendered feed must not
	// contain any of the arm vocabulary used elsewhere in SELAR.
	o, c := ts("2026-10-10T00:00:00Z"), ts("2026-10-17T00:00:00Z")
	var events []Event
	for _, k := range []quiz.Kind{quiz.KindInitial, quiz.KindFollowUp, quiz.KindPractice} {
		events = append(events, ToEvent(Window{QuizID: string(k), Kind: k, Start: o, End: c}, "6f1c1f0e-4b7a-4c39-9a51-0b7d2b1f8c11", "https://c"))
	}
	ics := strings.ToLower(Feed(events, *o))
	for _, banned := range []string{"control", "treatment", "hitl", "cohort", "group", "arm", "condition", "6f1c1f0e", "memory", "grade", "score"} {
		if strings.Contains(ics, banned) {
			t.Errorf("feed contains %q", banned)
		}
	}
	// Same input for two learners in different arms -> identical text apart
	// from the per-user UID.
	a := ToEvent(Window{QuizID: "q", Kind: quiz.KindInitial, Start: o, End: c}, "user-a", "https://c")
	b := ToEvent(Window{QuizID: "q", Kind: quiz.KindInitial, Start: o, End: c}, "user-b", "https://c")
	if a.Summary != b.Summary || a.Description != b.Description || a.UID == b.UID {
		t.Fatal("events must be identical across learners except the UID")
	}
}

func TestGoogleLinks(t *testing.T) {
	e := ToEvent(Window{QuizID: "q", Kind: quiz.KindInitial, Start: ts("2026-10-12T03:30:00Z"), End: ts("2026-10-12T12:00:00Z")}, "u", "https://c")
	u, err := url.Parse(GoogleTemplateURL(e))
	if err != nil || u.Host != "calendar.google.com" {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("action") != "TEMPLATE" || q.Get("dates") != "20261012T033000Z/20261012T120000Z" || q.Get("text") != "SELAR: Initial test window" {
		t.Fatalf("query = %v", q)
	}
	sub := GoogleSubscribeURL("https://api.example/calendar/abc.ics")
	if sub != "https://calendar.google.com/calendar/r?cid="+url.QueryEscape("webcal://api.example/calendar/abc.ics") {
		t.Fatalf("subscribe = %s", sub)
	}
}
