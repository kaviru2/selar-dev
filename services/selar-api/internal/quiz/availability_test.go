package quiz

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func ptr(t time.Time) *time.Time { return &t }

func published() Quiz {
	return Quiz{ID: "q1", Status: StatusPublished, Kind: KindPractice, Settings: Settings{MaxAttempts: 1, Feedback: FeedbackAfterSubmit, Audience: Audience{Type: AudienceAll}}}
}

func TestDraftQuizIsHiddenFromLearners(t *testing.T) {
	q := published()
	q.Status = StatusDraft
	if got := Evaluate(q, Learner{}, Attempts{}, t0).State; got != StateHidden {
		t.Fatalf("draft state = %s, want hidden", got)
	}
}

func TestAudienceRestrictsByCohortAndGroup(t *testing.T) {
	q := published()
	q.Settings.Audience = Audience{Type: AudienceCohort, Value: "control"}
	if got := Evaluate(q, Learner{Cohort: "treatment_hitl"}, Attempts{}, t0).State; got != StateHidden {
		t.Fatalf("other cohort sees quiz: %s", got)
	}
	if got := Evaluate(q, Learner{Cohort: "control"}, Attempts{}, t0).State; got != StateAvailable {
		t.Fatalf("matching cohort state = %s", got)
	}
	q.Settings.Audience = Audience{Type: AudienceGroup, Value: "pilot-a"}
	if got := Evaluate(q, Learner{Groups: []string{"x"}}, Attempts{}, t0).State; got != StateHidden {
		t.Fatalf("non-member sees group quiz: %s", got)
	}
	if got := Evaluate(q, Learner{Groups: []string{"pilot-a"}}, Attempts{}, t0).State; got != StateAvailable {
		t.Fatalf("member state = %s", got)
	}
}

func TestAbsoluteWindow(t *testing.T) {
	q := published()
	q.Settings.OpenAt = ptr(t0.Add(time.Hour))
	q.Settings.CloseAt = ptr(t0.Add(2 * time.Hour))
	r := Evaluate(q, Learner{}, Attempts{}, t0)
	if r.State != StateUpcoming || r.OpensAt == nil || !r.OpensAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("before open: %+v", r)
	}
	if got := Evaluate(q, Learner{}, Attempts{}, t0.Add(90*time.Minute)).State; got != StateAvailable {
		t.Fatalf("inside window: %s", got)
	}
	if got := Evaluate(q, Learner{}, Attempts{}, t0.Add(2*time.Hour)).State; got != StateMissed {
		t.Fatalf("after close without attempt: %s", got)
	}
	if got := Evaluate(q, Learner{}, Attempts{Used: 1}, t0.Add(3*time.Hour)).State; got != StateCompleted {
		t.Fatalf("after close with attempt: %s", got)
	}
}

func TestRelativeAvailabilityAfterFirstReading(t *testing.T) {
	q := published()
	q.Settings.After = &RelativeWindow{Anchor: AnchorFirstReading, Days: 7, WindowDays: 2}
	r := Evaluate(q, Learner{}, Attempts{}, t0)
	if r.State != StateUpcoming || r.OpensAt != nil || r.Reason == "" {
		t.Fatalf("anchor not reached should be upcoming without a date and with a reason: %+v", r)
	}
	lc := Learner{FirstReadingAt: ptr(t0)}
	r = Evaluate(q, lc, Attempts{}, t0.Add(6*24*time.Hour))
	if r.State != StateUpcoming || !r.OpensAt.Equal(t0.Add(7*24*time.Hour)) {
		t.Fatalf("day 6: %+v", r)
	}
	r = Evaluate(q, lc, Attempts{}, t0.Add(7*24*time.Hour))
	if r.State != StateAvailable || !r.ClosesAt.Equal(t0.Add(9*24*time.Hour)) {
		t.Fatalf("day 7: %+v", r)
	}
	if got := Evaluate(q, lc, Attempts{}, t0.Add(9*24*time.Hour)).State; got != StateMissed {
		t.Fatalf("after relative window: %s", got)
	}
}

func TestRelativeAvailabilityAfterDocumentAndQuiz(t *testing.T) {
	q := published()
	q.Settings.After = &RelativeWindow{Anchor: AnchorDocument, Document: "abc", Days: 1}
	lc := Learner{DocumentReadAt: map[string]time.Time{"abc": t0}}
	if got := Evaluate(q, lc, Attempts{}, t0.Add(25*time.Hour)).State; got != StateAvailable {
		t.Fatalf("document anchor: %s", got)
	}
	if got := Evaluate(q, Learner{DocumentReadAt: map[string]time.Time{"zzz": t0}}, Attempts{}, t0.Add(25*time.Hour)).State; got != StateUpcoming {
		t.Fatalf("other document must not anchor: %s", got)
	}
	q.Settings.After = &RelativeWindow{Anchor: AnchorQuiz, QuizID: "initial", Days: 7}
	lc = Learner{QuizSubmittedAt: map[string]time.Time{"initial": t0}}
	if got := Evaluate(q, lc, Attempts{}, t0.Add(7*24*time.Hour)).State; got != StateAvailable {
		t.Fatalf("quiz anchor: %s", got)
	}
}

func TestAbsoluteAndRelativeWindowsIntersect(t *testing.T) {
	q := published()
	q.Settings.CloseAt = ptr(t0.Add(3 * 24 * time.Hour))
	q.Settings.After = &RelativeWindow{Anchor: AnchorFirstReading, Days: 7}
	if got := Evaluate(q, Learner{FirstReadingAt: ptr(t0)}, Attempts{}, t0.Add(7*24*time.Hour)).State; got != StateMissed {
		t.Fatalf("relative opening after absolute close must be missed, got %s", got)
	}
}

func TestAttemptLimits(t *testing.T) {
	q := published()
	q.Settings.MaxAttempts = 2
	r := Evaluate(q, Learner{}, Attempts{Used: 1}, t0)
	if r.State != StateAvailable || r.AttemptsUsed != 1 || r.AttemptsAllowed != 2 {
		t.Fatalf("retake: %+v", r)
	}
	if got := Evaluate(q, Learner{}, Attempts{Used: 2}, t0).State; got != StateCompleted {
		t.Fatalf("exhausted: %s", got)
	}
	q.Settings.MaxAttempts = 0
	if got := Evaluate(q, Learner{}, Attempts{Used: 50}, t0).State; got != StateAvailable {
		t.Fatalf("unlimited: %s", got)
	}
	if got := Evaluate(q, Learner{}, Attempts{Used: 1, InProgress: true}, t0).State; got != StateInProgress {
		t.Fatalf("in progress: %s", got)
	}
}

func TestClosedStatusEndsAvailability(t *testing.T) {
	q := published()
	q.Status = StatusClosed
	if got := Evaluate(q, Learner{}, Attempts{}, t0).State; got != StateMissed {
		t.Fatalf("closed without attempt: %s", got)
	}
	if got := Evaluate(q, Learner{}, Attempts{Used: 1}, t0).State; got != StateCompleted {
		t.Fatalf("closed with attempt: %s", got)
	}
}

func TestFeedbackPolicy(t *testing.T) {
	q := published()
	q.Settings.Feedback = FeedbackNever
	if FeedbackVisible(q, true, t0) {
		t.Fatal("never policy exposed feedback")
	}
	q.Settings.Feedback = FeedbackAfterSubmit
	if FeedbackVisible(q, false, t0) || !FeedbackVisible(q, true, t0) {
		t.Fatal("after_submit policy wrong")
	}
	q.Settings.Feedback = FeedbackAfterClose
	q.Settings.CloseAt = ptr(t0.Add(time.Hour))
	if FeedbackVisible(q, true, t0) {
		t.Fatal("after_close exposed feedback before close")
	}
	if !FeedbackVisible(q, true, t0.Add(time.Hour)) {
		t.Fatal("after_close hid feedback after close_at")
	}
	q.Settings.CloseAt = nil
	q.Status = StatusClosed
	if !FeedbackVisible(q, true, t0) {
		t.Fatal("after_close hid feedback for a closed quiz")
	}
	if FeedbackVisible(q, false, t0) {
		t.Fatal("feedback shown for an unsubmitted attempt")
	}
}

func TestAttemptDeadline(t *testing.T) {
	q := published()
	if d := Deadline(q.Settings.TimeLimitSeconds, t0, q.Settings.CloseAt); d != nil {
		t.Fatalf("no limit and no close: %v", d)
	}
	q.Settings.TimeLimitSeconds = 600
	if d := Deadline(q.Settings.TimeLimitSeconds, t0, q.Settings.CloseAt); d == nil || !d.Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("time limit: %v", d)
	}
	q.Settings.CloseAt = ptr(t0.Add(5 * time.Minute))
	if d := Deadline(q.Settings.TimeLimitSeconds, t0, q.Settings.CloseAt); !d.Equal(t0.Add(5 * time.Minute)) {
		t.Fatalf("close_at caps deadline: %v", d)
	}
}
