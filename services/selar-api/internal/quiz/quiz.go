// Package quiz holds the storage-independent quiz rules: who may see a quiz,
// when it opens and closes for a learner, how answers are graded, and when
// feedback may be revealed. Handlers and the store call these functions so the
// rules are enforced on the server and unit-tested without a database.
package quiz

import "time"

type Kind string

const (
	KindInitial  Kind = "initial"
	KindFollowUp Kind = "follow_up"
	KindPractice Kind = "practice"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusClosed    Status = "closed"
)

type FeedbackPolicy string

const (
	FeedbackNever       FeedbackPolicy = "never"
	FeedbackAfterSubmit FeedbackPolicy = "after_submit"
	FeedbackAfterClose  FeedbackPolicy = "after_close"
)

type AudienceType string

const (
	AudienceAll    AudienceType = "all"
	AudienceCohort AudienceType = "cohort"
	AudienceGroup  AudienceType = "group"
)

type Audience struct {
	Type  AudienceType `json:"type"`
	Value string       `json:"value,omitempty"`
}

type Anchor string

const (
	AnchorFirstReading Anchor = "first_reading"
	AnchorDocument     Anchor = "document"
	AnchorQuiz         Anchor = "quiz_submitted"
)

// RelativeWindow opens a quiz Days after a per-learner anchor event and,
// when WindowDays > 0, closes it WindowDays later.
type RelativeWindow struct {
	Anchor     Anchor `json:"anchor"`
	Days       int    `json:"days"`
	WindowDays int    `json:"window_days,omitempty"`
	// Document matches a document's content hash or exact title for AnchorDocument.
	Document string `json:"document,omitempty"`
	// QuizID is the quiz whose first submission anchors AnchorQuiz.
	QuizID string `json:"quiz_id,omitempty"`
}

// Settings are the delivery rules stored with a quiz.
type Settings struct {
	OpenAt           *time.Time      `json:"open_at,omitempty"`
	CloseAt          *time.Time      `json:"close_at,omitempty"`
	After            *RelativeWindow `json:"after,omitempty"`
	TimeLimitSeconds int             `json:"time_limit_seconds,omitempty"`
	MaxAttempts      int             `json:"max_attempts"` // 0 = unlimited
	Feedback         FeedbackPolicy  `json:"feedback"`
	Audience         Audience        `json:"audience"`
	ShuffleQuestions bool            `json:"shuffle_questions"`
	ShuffleOptions   bool            `json:"shuffle_options"`
	NoGoingBack      bool            `json:"no_going_back"`
}

type Quiz struct {
	ID       string
	Status   Status
	Kind     Kind
	Settings Settings
}

// Learner carries the per-user facts availability depends on.
type Learner struct {
	Cohort          string
	Groups          []string
	FirstReadingAt  *time.Time
	DocumentReadAt  map[string]time.Time // keyed by content hash and by title
	QuizSubmittedAt map[string]time.Time
}

type Attempts struct {
	Used       int // attempts started, submitted or not
	InProgress bool
}

type State string

const (
	StateHidden     State = "hidden"
	StateUpcoming   State = "upcoming"
	StateAvailable  State = "available"
	StateInProgress State = "in_progress"
	StateCompleted  State = "completed"
	StateMissed     State = "missed"
)

type Availability struct {
	State           State      `json:"state"`
	OpensAt         *time.Time `json:"opens_at,omitempty"`
	ClosesAt        *time.Time `json:"closes_at,omitempty"`
	Reason          string     `json:"reason,omitempty"`
	AttemptsUsed    int        `json:"attempts_used"`
	AttemptsAllowed int        `json:"attempts_allowed"`
}

// InAudience reports whether the learner is targeted by the quiz.
func InAudience(a Audience, l Learner) bool {
	switch a.Type {
	case AudienceCohort:
		return a.Value != "" && l.Cohort == a.Value
	case AudienceGroup:
		for _, g := range l.Groups {
			if g == a.Value && a.Value != "" {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// Window returns the learner's effective open and close times. ok is false
// when a relative anchor has not happened yet, so no opening time exists.
func Window(s Settings, l Learner) (open, close *time.Time, ok bool, reason string) {
	open, close = s.OpenAt, s.CloseAt
	if s.After == nil {
		return open, close, true, ""
	}
	var anchor *time.Time
	switch s.After.Anchor {
	case AnchorFirstReading:
		anchor = l.FirstReadingAt
		reason = "Opens after your first reading session"
	case AnchorDocument:
		if at, found := l.DocumentReadAt[s.After.Document]; found {
			anchor = &at
		}
		reason = "Opens after you read the linked document"
	case AnchorQuiz:
		if at, found := l.QuizSubmittedAt[s.After.QuizID]; found {
			anchor = &at
		}
		reason = "Opens after you complete an earlier quiz"
	}
	if anchor == nil {
		return nil, close, false, reason
	}
	relOpen := anchor.Add(time.Duration(s.After.Days) * 24 * time.Hour)
	if open == nil || relOpen.After(*open) {
		open = &relOpen
	}
	if s.After.WindowDays > 0 {
		relClose := relOpen.Add(time.Duration(s.After.WindowDays) * 24 * time.Hour)
		if close == nil || relClose.Before(*close) {
			close = &relClose
		}
	}
	return open, close, true, ""
}

// Evaluate decides what a learner may do with a quiz at time now.
func Evaluate(q Quiz, l Learner, a Attempts, now time.Time) Availability {
	r := Availability{AttemptsUsed: a.Used, AttemptsAllowed: q.Settings.MaxAttempts}
	if q.Status == StatusDraft || !InAudience(q.Settings.Audience, l) {
		r.State = StateHidden
		return r
	}
	finished := func() Availability {
		if a.Used > 0 {
			r.State = StateCompleted
		} else {
			r.State = StateMissed
		}
		return r
	}
	open, close, ok, reason := Window(q.Settings, l)
	r.OpensAt, r.ClosesAt = open, close
	closedNow := q.Status == StatusClosed || (close != nil && !now.Before(*close))
	if a.InProgress && !closedNow {
		r.State = StateInProgress
		return r
	}
	if closedNow {
		return finished()
	}
	if !ok {
		r.State, r.Reason = StateUpcoming, reason
		return r
	}
	if open != nil && now.Before(*open) {
		r.State = StateUpcoming
		return r
	}
	if q.Settings.MaxAttempts > 0 && a.Used >= q.Settings.MaxAttempts {
		r.State = StateCompleted
		return r
	}
	r.State = StateAvailable
	return r
}

// FeedbackVisible reports whether correctness, keys and scores may be shown
// to the learner for a submitted attempt.
func FeedbackVisible(q Quiz, submitted bool, now time.Time) bool {
	return FeedbackVisibleWithClose(q, submitted, now, q.Settings.CloseAt)
}

// FeedbackVisibleWithClose is FeedbackVisible with the learner's effective
// close time (which may come from a relative window).
func FeedbackVisibleWithClose(q Quiz, submitted bool, now time.Time, close *time.Time) bool {
	if !submitted {
		return false
	}
	switch q.Settings.Feedback {
	case FeedbackAfterSubmit:
		return true
	case FeedbackAfterClose:
		return q.Status == StatusClosed || (close != nil && !now.Before(*close))
	default:
		return false
	}
}

// Deadline is when an attempt started at startedAt must be submitted: the
// earlier of the time limit and the quiz's close time. nil means no deadline.
func Deadline(timeLimitSeconds int, startedAt time.Time, close *time.Time) *time.Time {
	var d *time.Time
	if timeLimitSeconds > 0 {
		v := startedAt.Add(time.Duration(timeLimitSeconds) * time.Second)
		d = &v
	}
	if close != nil && (d == nil || close.Before(*d)) {
		v := *close
		d = &v
	}
	return d
}
