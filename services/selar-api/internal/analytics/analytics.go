// Package analytics defines SELAR's first-party usage-event allow-list.
//
// Nothing here talks to a third party. An event is accepted only if its name
// is in the dictionary below, and only the properties declared for that event
// survive: every value is type-checked (UUID, enum, bounded integer, boolean
// or same-origin route) so free text such as chat questions, explanations,
// passages or emails can never be stored through this path.
//
// docs/ANALYTICS.md is the human-readable copy of this dictionary; a test
// keeps the two in sync.
package analytics

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ConsentVersion identifies the wording of the optional research-consent
// checkbox. Bump it whenever that wording changes so earlier consents can be
// told apart (users.consent_version stores the version they agreed to).
const ConsentVersion = "prototype-usage-v1"

// Event is an allow-listed analytics event name.
type Event string

// Props are event properties before or after sanitising.
type Props map[string]any

// Source says who is allowed to emit an event. Server events are facts the
// API observed itself ("server-side truth") and are refused from browsers.
type Source string

const (
	SourceServer Source = "server"
	SourceClient Source = "client"
)

// Server-side events (emitted by the Go API).
const (
	SignedIn             Event = "signed_in"
	DocumentUploaded     Event = "document_uploaded"
	ReadingSessionStart  Event = "reading_session_started"
	ReadingSessionEnd    Event = "reading_session_ended"
	HighlightCreated     Event = "highlight_created"
	DecisionMade         Event = "decision_made"
	ChatQuestionAsked    Event = "chat_question_asked"
	ChatCitationOpened   Event = "chat_citation_opened"
	QuizStarted          Event = "quiz_started"
	QuizSubmitted        Event = "quiz_submitted"
	ConsentGranted       Event = "research_consent_granted"
	AnalyticsDataDeleted Event = "analytics_data_deleted"
)

// Client-side events (UI-only actions, sent by the console in batches).
const (
	AppSessionStarted Event = "app_session_started"
	AppSessionEnded   Event = "app_session_ended"
	PageViewed        Event = "page_viewed"
	ReaderPageViewed  Event = "reader_page_viewed"
	SuggestionShown   Event = "suggestion_shown"
	SuggestionOpened  Event = "suggestion_opened"
	ExplainSubmitted  Event = "explain_submitted"
	CompareViewed     Event = "compare_viewed"
)

// Limits shared with the console client and the docs.
const (
	MaxDwellMs    = 30 * 60 * 1000 // a single page view longer than 30 min is treated as idle, not reading
	MaxSessionMs  = 12 * 60 * 60 * 1000
	MaxTextLength = 20000
	MaxRouteLen   = 120
	MaxBatch      = 50
	MaxClientSkew = 24 * time.Hour
)

type kind int

const (
	kindUUID kind = iota
	kindEnum
	kindInt
	kindBool
	kindRoute
)

// PropSpec declares one allowed property.
type PropSpec struct {
	Kind     string   `json:"kind"`
	Enum     []string `json:"enum,omitempty"`
	Min      int64    `json:"min,omitempty"`
	Max      int64    `json:"max,omitempty"`
	internal kind
}

// Spec documents one event.
type Spec struct {
	Name        Event               `json:"name"`
	Source      Source              `json:"source"`
	Description string              `json:"description"`
	Props       map[string]PropSpec `json:"props"`
	// CountOnly events are never stored per user, only as anonymous daily counts.
	CountOnly bool `json:"count_only,omitempty"`
}

func id() PropSpec { return PropSpec{Kind: "uuid", internal: kindUUID} }
func enum(values ...string) PropSpec {
	return PropSpec{Kind: "enum", Enum: values, internal: kindEnum}
}
func integer(min, max int64) PropSpec {
	return PropSpec{Kind: "int", Min: min, Max: max, internal: kindInt}
}
func boolean() PropSpec { return PropSpec{Kind: "bool", internal: kindBool} }
func route() PropSpec   { return PropSpec{Kind: "route", internal: kindRoute} }

var (
	decisionKinds = []string{"mental_link", "passage_link", "concept", "concept_edge"}
	quizPhases    = []string{"initial", "follow_up", "practice"}
)

var dictionary = map[Event]Spec{
	SignedIn: {Source: SourceServer, Description: "A login or registration succeeded (start of an authenticated session).",
		Props: map[string]PropSpec{"method": enum("login", "register")}},
	DocumentUploaded: {Source: SourceServer, Description: "A document was accepted and queued for ingestion.",
		Props: map[string]PropSpec{"document_id": id(), "source_type": enum("pdf", "web", "text")}},
	ReadingSessionStart: {Source: SourceServer, Description: "The reader opened a document (reading_sessions row created).",
		Props: map[string]PropSpec{"document_id": id(), "session_id": id()}},
	ReadingSessionEnd: {Source: SourceServer, Description: "A reading session ended; duration is computed by the server from its start time.",
		Props: map[string]PropSpec{"document_id": id(), "session_id": id(), "duration_ms": integer(0, MaxSessionMs),
			"pages_viewed": integer(0, 100000), "max_scroll_depth": integer(0, 100)}},
	HighlightCreated: {Source: SourceServer, Description: "A highlight, underline or note was saved (note text is not recorded).",
		Props: map[string]PropSpec{"document_id": id(), "page": integer(0, 100000), "type": enum("highlight", "underline", "note", "suggestion")}},
	DecisionMade: {Source: SourceServer, Description: "The user decided on a suggestion: keep, change (relabel), reject, retract or undo.",
		Props: map[string]PropSpec{"kind": enum(decisionKinds...), "action": enum("keep", "change", "reject", "retract", "undo"),
			"link_id": id(), "response_ms": integer(0, 24*60*60*1000)}},
	ChatQuestionAsked: {Source: SourceServer, Description: "A grounded-chat question was sent (length only, never the text).",
		Props: map[string]PropSpec{"thread_id": id(), "length": integer(0, MaxTextLength)}},
	ChatCitationOpened: {Source: SourceServer, Description: "A chat citation was opened.",
		Props: map[string]PropSpec{"citation_id": id()}},
	QuizStarted: {Source: SourceServer, Description: "A quiz attempt started (emitted by the quiz feature through Track).",
		Props: map[string]PropSpec{"quiz_id": id(), "attempt_id": id(), "phase": enum(quizPhases...), "question_count": integer(0, 1000)}},
	QuizSubmitted: {Source: SourceServer, Description: "A quiz attempt was submitted and scored (score as a 0–100 percentage).",
		Props: map[string]PropSpec{"quiz_id": id(), "attempt_id": id(), "phase": enum(quizPhases...), "question_count": integer(0, 1000),
			"score_pct": integer(0, 100), "duration_ms": integer(0, MaxSessionMs)}},
	ConsentGranted: {Source: SourceServer, Description: "The user opted in to usage analytics for research.",
		Props: map[string]PropSpec{"via": enum("register", "prompt", "settings")}},
	AnalyticsDataDeleted: {Source: SourceServer, Description: "Anonymous counter only: a user deleted their analytics events.",
		Props: map[string]PropSpec{}, CountOnly: true},

	AppSessionStarted: {Source: SourceClient, Description: "The console was opened in a browser tab.",
		Props: map[string]PropSpec{"route": route()}},
	AppSessionEnded: {Source: SourceClient, Description: "The tab was hidden or closed; duration counts visible time only.",
		Props: map[string]PropSpec{"duration_ms": integer(0, MaxSessionMs)}},
	PageViewed: {Source: SourceClient, Description: "A console route was shown (path only, query string stripped).",
		Props: map[string]PropSpec{"route": route()}},
	ReaderPageViewed: {Source: SourceClient, Description: "Time spent on one reader page, sent when the reader leaves it (min 1 s, capped at 30 min).",
		Props: map[string]PropSpec{"document_id": id(), "page": integer(0, 100000), "dwell_ms": integer(0, MaxDwellMs)}},
	SuggestionShown: {Source: SourceClient, Description: "A suggested connection was shown to the user.",
		Props: map[string]PropSpec{"kind": enum(decisionKinds...), "link_id": id(), "position": integer(0, 1000)}},
	SuggestionOpened: {Source: SourceClient, Description: "The user moved from noticing a suggestion to thinking about it.",
		Props: map[string]PropSpec{"kind": enum(decisionKinds...), "link_id": id()}},
	ExplainSubmitted: {Source: SourceClient, Description: "The user left the explain step; only character counts are sent, never the text.",
		Props: map[string]PropSpec{"link_id": id(), "length": integer(0, MaxTextLength), "recall_length": integer(0, MaxTextLength), "recall_on": boolean()}},
	CompareViewed: {Source: SourceClient, Description: "The source passages for a suggestion were shown side by side.",
		Props: map[string]PropSpec{"kind": enum(decisionKinds...), "link_id": id()}},
}

// Dictionary returns every allow-listed event sorted by name.
func Dictionary() []Spec {
	out := make([]Spec, 0, len(dictionary))
	for name, spec := range dictionary {
		spec.Name = name
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup returns the spec for an event name.
func Lookup(event Event) (Spec, bool) {
	spec, ok := dictionary[event]
	spec.Name = event
	return spec, ok
}

// ErrUnknownEvent is returned for names outside the allow-list.
var ErrUnknownEvent = errors.New("unknown analytics event")

// ErrWrongSource is returned when a browser sends a server-side event.
var ErrWrongSource = errors.New("event may only be emitted by the server")

// Sanitize validates an event and returns only its allow-listed, well-typed
// properties. Invalid individual properties are dropped (the event is kept).
// Server code may emit any event; clients only client events.
func Sanitize(event Event, props Props, source Source) (Props, error) {
	spec, ok := dictionary[event]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEvent, event)
	}
	if source == SourceClient && spec.Source != SourceClient {
		return nil, fmt.Errorf("%w: %q", ErrWrongSource, event)
	}
	clean := Props{}
	for key, value := range props {
		prop, ok := spec.Props[key]
		if !ok {
			continue
		}
		if normalized, ok := normalize(prop, value); ok {
			clean[key] = normalized
		}
	}
	return clean, nil
}

func normalize(prop PropSpec, value any) (any, bool) {
	switch prop.internal {
	case kindUUID:
		s, ok := value.(string)
		if !ok {
			return nil, false
		}
		parsed, err := uuid.Parse(s)
		if err != nil {
			return nil, false
		}
		return parsed.String(), true
	case kindEnum:
		s, ok := value.(string)
		if !ok {
			return nil, false
		}
		for _, allowed := range prop.Enum {
			if s == allowed {
				return s, true
			}
		}
		return nil, false
	case kindInt:
		n, ok := toInt(value)
		if !ok || n < prop.Min || n > prop.Max {
			return nil, false
		}
		return n, true
	case kindBool:
		b, ok := value.(bool)
		return b, ok
	case kindRoute:
		s, ok := value.(string)
		if !ok {
			return nil, false
		}
		if i := strings.IndexAny(s, "?#"); i >= 0 {
			s = s[:i]
		}
		if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || len(s) > MaxRouteLen {
			return nil, false
		}
		for _, r := range s {
			if !(r == '/' || r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
				return nil, false
			}
		}
		return s, true
	}
	return nil, false
}

func toInt(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case float32:
		return toInt(float64(v))
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return int64(math.Round(v)), true
	}
	return 0, false
}

// ClampClientTime keeps a browser-reported timestamp only when it is
// plausible (not in the future, at most MaxClientSkew old); otherwise the
// server receive time is used.
func ClampClientTime(reported, now time.Time) time.Time {
	if reported.IsZero() || reported.After(now.Add(time.Minute)) || reported.Before(now.Add(-MaxClientSkew)) {
		return now
	}
	return reported
}

// EmailSet is a case-insensitive set of email addresses (ADMIN_EMAILS).
type EmailSet map[string]struct{}

// ParseEmailList parses a comma-separated list such as ADMIN_EMAILS.
func ParseEmailList(raw string) EmailSet {
	set := EmailSet{}
	for _, part := range strings.Split(raw, ",") {
		if email := strings.ToLower(strings.TrimSpace(part)); email != "" {
			set[email] = struct{}{}
		}
	}
	return set
}

// Contains reports whether email is in the set (case-insensitive).
func (s EmailSet) Contains(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	_, ok := s[email]
	return ok
}
