// Package calendar builds the optional per-learner quiz-window calendar feed
// (RFC 5545 iCalendar) and the "Add to Google Calendar" template links.
//
// Design rules (issue #113):
//
//   - Windows come from quiz.Evaluate / quiz.Window, the same functions the
//     quiz API uses to enforce availability, so the calendar can never show a
//     window the server would not honour.
//   - Every time is written in UTC ("…Z"). Calendar clients convert to the
//     viewer's zone, so Asia/Colombo (UTC+05:30) and every other zone render
//     the exact server-enforced instant. No floating or TZID times are used.
//   - Event text is the same for every participant: the quiz kind, the times
//     and a link to SELAR. Quiz titles, descriptions, cohorts and group labels
//     are deliberately left out so nothing in a calendar reveals a study arm.
package calendar

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/selar-dev/selar-api/internal/quiz"
)

// Window is one quiz window for one learner, already evaluated by the quiz
// rules. Start is nil when the quiz is open now without a fixed start (the
// event then marks the close); End is nil when the window never closes.
type Window struct {
	QuizID  string
	Kind    quiz.Kind
	Start   *time.Time
	End     *time.Time
	Updated time.Time
}

// FromAvailability turns an evaluated quiz into a calendar window. Only
// windows a learner can still act on are kept: upcoming (with a known start),
// available and in progress. Hidden, completed and missed quizzes, and
// relative windows whose anchor has not happened yet, produce no event.
func FromAvailability(quizID string, kind quiz.Kind, a quiz.Availability, updated time.Time) (Window, bool) {
	switch a.State {
	case quiz.StateUpcoming:
		if a.OpensAt == nil {
			return Window{}, false
		}
	case quiz.StateAvailable, quiz.StateInProgress:
	default:
		return Window{}, false
	}
	if a.OpensAt == nil && a.ClosesAt == nil {
		return Window{}, false
	}
	return Window{QuizID: quizID, Kind: kind, Start: utc(a.OpensAt), End: utc(a.ClosesAt), Updated: updated.UTC()}, true
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC().Truncate(time.Second)
	return &v
}

// KindLabel is the neutral, arm-independent name used in events.
func KindLabel(k quiz.Kind) string {
	switch k {
	case quiz.KindInitial:
		return "Initial test"
	case quiz.KindFollowUp:
		return "Follow-up test"
	case quiz.KindPractice:
		return "Practice quiz"
	}
	return "Quiz"
}

// Event is the VEVENT shape of a window.
type Event struct {
	UID         string
	Summary     string
	Description string
	Start, End  time.Time
	Updated     time.Time
}

const markerLength = 30 * time.Minute

// ToEvent maps a window to one event. A closed range becomes one event
// spanning the window, marked free (TRANSP:TRANSPARENT) so a week-long window
// does not block the learner's calendar. A window with no close becomes a
// short "opens" marker; a window with no start becomes a short "closes"
// marker ending at the close time.
func ToEvent(w Window, userID, consoleURL string) Event {
	label := KindLabel(w.Kind)
	link := QuizzesURL(consoleURL)
	e := Event{UID: EventUID(userID, w.QuizID), Updated: w.Updated}
	switch {
	case w.Start != nil && w.End != nil:
		e.Start, e.End = *w.Start, *w.End
		e.Summary = "SELAR: " + label + " window"
		e.Description = "This SELAR " + strings.ToLower(label) + " can be taken between the start and end of this event. " +
			"Times follow the SELAR server and are shown in your calendar's time zone."
	case w.Start != nil:
		e.Start, e.End = *w.Start, w.Start.Add(markerLength)
		e.Summary = "SELAR: " + label + " opens"
		e.Description = "This SELAR " + strings.ToLower(label) + " opens at the start of this event and has no closing time."
	default:
		e.Start, e.End = w.End.Add(-markerLength), *w.End
		e.Summary = "SELAR: " + label + " closes"
		e.Description = "This SELAR " + strings.ToLower(label) + " is open now and closes at the end of this event."
	}
	if !e.End.After(e.Start) {
		e.End = e.Start.Add(time.Minute)
	}
	e.Description += "\n\nOpen SELAR: " + link + "\nSELAR is a research prototype. You can turn this calendar off in SELAR Settings."
	return e
}

// EventUID is stable for a (learner, quiz) pair, so a changed window updates
// the existing event instead of adding a new one. The user id is hashed so a
// shared or forwarded event does not expose it.
func EventUID(userID, quizID string) string {
	sum := sha256.Sum256([]byte("selar-calendar-uid\x00" + userID + "\x00" + quizID))
	return "quiz-" + hex.EncodeToString(sum[:12]) + "@selar"
}

// QuizzesURL is the learner quiz list on the console.
func QuizzesURL(consoleURL string) string {
	return strings.TrimRight(consoleURL, "/") + "/quizzes"
}

// Feed renders a complete VCALENDAR. now is the DTSTAMP. Events are sorted
// by start time, then UID, so the output is deterministic.
func Feed(events []Event, now time.Time) string {
	sort.Slice(events, func(i, j int) bool {
		if !events[i].Start.Equal(events[j].Start) {
			return events[i].Start.Before(events[j].Start)
		}
		return events[i].UID < events[j].UID
	})
	var b strings.Builder
	line := func(s string) { b.WriteString(fold(s)) }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//SELAR research prototype//Quiz windows//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:SELAR quiz windows")
	line("X-WR-CALDESC:" + escape("Times when your SELAR quizzes are open. SELAR is a research prototype."))
	line("REFRESH-INTERVAL;VALUE=DURATION:PT1H")
	line("X-PUBLISHED-TTL:PT1H")
	stamp := stampFmt(now)
	for _, e := range events {
		line("BEGIN:VEVENT")
		line("UID:" + escape(e.UID))
		line("DTSTAMP:" + stamp)
		line("DTSTART:" + stampFmt(e.Start))
		line("DTEND:" + stampFmt(e.End))
		if !e.Updated.IsZero() {
			line("LAST-MODIFIED:" + stampFmt(e.Updated))
			// Bump SEQUENCE whenever the quiz changes so clients that honour
			// it replace the old version of the event.
			line(fmt.Sprintf("SEQUENCE:%d", e.Updated.Unix()/60%1_000_000_000))
		}
		line("SUMMARY:" + escape(e.Summary))
		line("DESCRIPTION:" + escape(e.Description))
		line("TRANSP:TRANSPARENT")
		line("STATUS:CONFIRMED")
		line("CLASS:PRIVATE")
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String()
}

func stampFmt(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// escape applies RFC 5545 §3.3.11 TEXT escaping.
func escape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return r.Replace(s)
}

// fold writes one content line folded at 75 octets (RFC 5545 §3.1) without
// splitting a UTF-8 sequence, terminated by CRLF.
func fold(s string) string {
	const limit = 75
	var b strings.Builder
	width := 0
	for _, r := range s {
		n := len(string(r))
		if width+n > limit {
			b.WriteString("\r\n ")
			width = 1
		}
		b.WriteRune(r)
		width += n
	}
	b.WriteString("\r\n")
	return b.String()
}

// GoogleTemplateURL is an "Add to Google Calendar" link for one event. It
// is a snapshot: unlike the feed it does not change if the window changes.
func GoogleTemplateURL(e Event) string {
	q := url.Values{}
	q.Set("action", "TEMPLATE")
	q.Set("text", e.Summary)
	q.Set("dates", stampFmt(e.Start)+"/"+stampFmt(e.End))
	q.Set("details", e.Description)
	return "https://calendar.google.com/calendar/render?" + q.Encode()
}

// GoogleSubscribeURL opens Google Calendar's "add by URL" flow for the feed.
func GoogleSubscribeURL(feedURL string) string {
	webcal := feedURL
	if i := strings.Index(webcal, "://"); i >= 0 {
		webcal = "webcal" + webcal[i:]
	}
	return "https://calendar.google.com/calendar/r?cid=" + url.QueryEscape(webcal)
}
