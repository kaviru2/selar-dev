package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/quiz"
)

type quizFixture struct {
	s       *Store
	ctx     context.Context
	admin   string
	learner string
	other   string
}

func newQuizFixture(t *testing.T) *quizFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	f := &quizFixture{s: New(pool), ctx: ctx}
	mk := func(cohort string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,cohort) VALUES ('selar.qa+'||gen_random_uuid()::text||'@example.com','x',$1) RETURNING id`, cohort).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.admin, f.learner, f.other = mk("control"), mk("control"), mk("treatment_hitl")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM quizzes WHERE created_by=$1`, f.admin)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1)`, []string{f.admin, f.learner, f.other})
		pool.Close()
	})
	return f
}

func demoDraft() quiz.Draft {
	d := quiz.Draft{
		Title: "DEMO quiz", Kind: quiz.KindPractice,
		Settings: quiz.Settings{MaxAttempts: 1, Feedback: quiz.FeedbackAfterSubmit},
		Questions: []quiz.Question{
			{Type: quiz.TypeSingleChoice, Prompt: "Pick B", Points: 2, Options: []quiz.Option{{Text: "A"}, {Text: "B", Correct: true}}, Explanation: "B is right"},
			{Type: quiz.TypeShortAnswer, Prompt: "Name the valley", Points: 1, AcceptedAnswers: []string{"Vell"}},
			{Type: quiz.TypeFreeRecall, Prompt: "Recall", Points: 5, Rubric: "secret rubric"},
		},
	}
	if err := d.Normalise(); err != nil {
		panic(err)
	}
	return d
}

func (f *quizFixture) publish(t *testing.T, d quiz.Draft) string {
	t.Helper()
	id, err := f.s.CreateQuiz(f.ctx, f.admin, d)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.SetQuizStatus(f.ctx, id, quiz.StatusPublished); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestQuizLearnerFlowEnforcesPolicyAndHidesKeys(t *testing.T) {
	f := newQuizFixture(t)
	now := time.Now()
	draftID, err := f.s.CreateQuiz(f.ctx, f.admin, demoDraft())
	if err != nil {
		t.Fatal(err)
	}
	cards, err := f.s.ListQuizzesForLearner(f.ctx, f.learner, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cards {
		if c.ID == draftID {
			t.Fatal("draft quiz listed for learner")
		}
	}
	if _, err := f.s.StartQuizAttempt(f.ctx, f.learner, draftID, now); !errors.Is(err, ErrQuizUnavailable) {
		t.Fatalf("start draft: %v", err)
	}
	if err := f.s.SetQuizStatus(f.ctx, draftID, quiz.StatusPublished); err != nil {
		t.Fatal(err)
	}
	view, err := f.s.StartQuizAttempt(f.ctx, f.learner, draftID, now)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.s.StartQuizAttempt(f.ctx, f.learner, draftID, now)
	if err != nil || again.AttemptID != view.AttemptID {
		t.Fatalf("starting twice must resume the open attempt: %v %v", again, err)
	}
	if len(view.Questions) != 3 {
		t.Fatalf("questions: %d", len(view.Questions))
	}
	raw := strings.ToLower(mustJSON(t, view))
	for _, leak := range []string{"secret rubric", "b is right", "accepted", "\"correct\"", "vell\""} {
		if strings.Contains(raw, leak) {
			t.Fatalf("learner attempt view leaks %q: %s", leak, raw)
		}
	}
	sc := view.Questions[0]
	if err := f.s.SaveQuizAnswer(f.ctx, f.other, view.AttemptID, SaveAnswerInput{QuestionID: sc.ID, Selected: []string{sc.Options[1].ID}}, now); !errors.Is(err, ErrAttemptNotFound) {
		t.Fatalf("other user saved into attempt: %v", err)
	}
	if err := f.s.SaveQuizAnswer(f.ctx, f.learner, view.AttemptID, SaveAnswerInput{QuestionID: sc.ID, Selected: []string{"nope"}}, now); !errors.Is(err, ErrInvalidAnswer) {
		t.Fatalf("unknown option accepted: %v", err)
	}
	for _, in := range []SaveAnswerInput{
		{QuestionID: sc.ID, Selected: []string{"o2"}, TimeOnQuestionMs: 4000},
		{QuestionID: view.Questions[1].ID, Text: " vell ", TimeOnQuestionMs: 999999999},
		{QuestionID: view.Questions[2].ID, Text: "Everything I recall"},
	} {
		if err := f.s.SaveQuizAnswer(f.ctx, f.learner, view.AttemptID, in, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	var capped int64
	if err := f.s.pool.QueryRow(f.ctx, `SELECT time_on_question_ms FROM quiz_answers WHERE attempt_id=$1 AND question_id=$2`, view.AttemptID, view.Questions[1].ID).Scan(&capped); err != nil || capped > 60_000 {
		t.Fatalf("time on question not capped by wall time: %d %v", capped, err)
	}
	if _, err := f.s.QuizResult(f.ctx, f.learner, view.AttemptID, now); !errors.Is(err, ErrAttemptNotSubmitted) {
		t.Fatalf("result before submit: %v", err)
	}
	if err := f.s.SubmitQuizAttempt(f.ctx, f.learner, view.AttemptID, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SubmitQuizAttempt(f.ctx, f.learner, view.AttemptID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("submit must be idempotent: %v", err)
	}
	if err := f.s.SaveQuizAnswer(f.ctx, f.learner, view.AttemptID, SaveAnswerInput{QuestionID: sc.ID, Selected: []string{"o1"}}, now); !errors.Is(err, ErrAttemptSubmitted) {
		t.Fatalf("saved after submit: %v", err)
	}
	if _, err := f.s.pool.Exec(f.ctx, `UPDATE quiz_answers SET answer_text='tamper' WHERE attempt_id=$1`, view.AttemptID); err == nil {
		t.Fatal("database allowed editing a submitted answer")
	}
	res, err := f.s.QuizResult(f.ctx, f.learner, view.AttemptID, now)
	if err != nil {
		t.Fatal(err)
	}
	if !res.FeedbackAvailable || res.Summary == nil || res.Summary.AutoPoints != 3 || res.Summary.PendingReview != 1 || res.Summary.Score != nil {
		t.Fatalf("result summary: %+v", res.Summary)
	}
	if _, err := f.s.StartQuizAttempt(f.ctx, f.learner, draftID, now); !errors.Is(err, ErrQuizUnavailable) {
		t.Fatalf("second attempt beyond max_attempts: %v", err)
	}

	// Manual grading completes the score.
	detail, err := f.s.GetAttemptForAdmin(f.ctx, view.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	var recallAnswer string
	for _, a := range detail.Answers {
		if a.NeedsReview {
			recallAnswer = a.ID
		}
	}
	over := 9.0
	if err := f.s.GradeQuizAnswer(f.ctx, f.admin, recallAnswer, &over, ""); !errors.Is(err, ErrInvalidGrade) {
		t.Fatalf("score above points accepted: %v", err)
	}
	four := 4.0
	if err := f.s.GradeQuizAnswer(f.ctx, f.admin, recallAnswer, &four, "good recall"); err != nil {
		t.Fatal(err)
	}
	res, _ = f.s.QuizResult(f.ctx, f.learner, view.AttemptID, now)
	if res.Summary.Score == nil || *res.Summary.Score != 7 || res.Summary.PendingReview != 0 {
		t.Fatalf("after grading: %+v", res.Summary)
	}
	// The admin view must stop flagging a graded answer as needing review,
	// otherwise the grading UI keeps offering it.
	graded, err := f.s.GetAttemptForAdmin(f.ctx, view.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range graded.Answers {
		if a.NeedsReview {
			t.Fatalf("answer %s still needs review after grading: %+v", a.QuestionID, a)
		}
	}
	rows, err := f.s.QuizExportRows(f.ctx, draftID)
	if err != nil || len(rows) != 3 {
		t.Fatalf("export rows: %d %v", len(rows), err)
	}
}

func TestQuizFeedbackNeverHidesScoreAndKeys(t *testing.T) {
	f := newQuizFixture(t)
	d := demoDraft()
	d.Settings.Feedback = quiz.FeedbackNever
	id := f.publish(t, d)
	now := time.Now()
	v, err := f.s.StartQuizAttempt(f.ctx, f.learner, id, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.SubmitQuizAttempt(f.ctx, f.learner, v.AttemptID, now); err != nil {
		t.Fatal(err)
	}
	res, err := f.s.QuizResult(f.ctx, f.learner, v.AttemptID, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.FeedbackAvailable || res.Summary != nil || len(res.Questions) != 0 {
		t.Fatalf("never policy leaked: %+v", res)
	}
}

func TestQuizTimeLimitAutoSubmitsAndRejectsLateAnswers(t *testing.T) {
	f := newQuizFixture(t)
	d := demoDraft()
	d.Settings.TimeLimitSeconds = 60
	id := f.publish(t, d)
	start := time.Now()
	v, err := f.s.StartQuizAttempt(f.ctx, f.learner, id, start)
	if err != nil {
		t.Fatal(err)
	}
	if v.DeadlineAt == nil || v.DeadlineAt.Sub(start) > 61*time.Second {
		t.Fatalf("deadline: %v", v.DeadlineAt)
	}
	late := start.Add(5 * time.Minute)
	err = f.s.SaveQuizAnswer(f.ctx, f.learner, v.AttemptID, SaveAnswerInput{QuestionID: v.Questions[0].ID, Selected: []string{"o2"}}, late)
	if !errors.Is(err, ErrAttemptSubmitted) {
		t.Fatalf("late save: %v", err)
	}
	var reason string
	if err := f.s.pool.QueryRow(f.ctx, `SELECT submit_reason FROM quiz_attempts WHERE id=$1`, v.AttemptID).Scan(&reason); err != nil || reason != "time_limit" {
		t.Fatalf("auto submit reason %q %v", reason, err)
	}
}

func TestQuizNoGoingBackServesOnlyCurrentQuestion(t *testing.T) {
	f := newQuizFixture(t)
	d := demoDraft()
	d.Settings.NoGoingBack = true
	id := f.publish(t, d)
	now := time.Now()
	v, err := f.s.StartQuizAttempt(f.ctx, f.learner, id, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Questions) != 1 || v.Total != 3 || v.CurrentPosition != 0 {
		t.Fatalf("no-going-back view must contain only the current question: %+v", v)
	}
	first := v.Questions[0].ID
	v, err = f.s.AdvanceQuizAttempt(f.ctx, f.learner, v.AttemptID, now)
	if err != nil || v.CurrentPosition != 1 || v.Questions[0].ID == first {
		t.Fatalf("advance: %+v %v", v, err)
	}
	if err := f.s.SaveQuizAnswer(f.ctx, f.learner, v.AttemptID, SaveAnswerInput{QuestionID: first, Selected: []string{"o1"}}, now); !errors.Is(err, ErrNoGoingBack) {
		t.Fatalf("answered a previous question: %v", err)
	}
}

func TestQuizAudienceWindowAndRelativeAnchor(t *testing.T) {
	f := newQuizFixture(t)
	now := time.Now()
	d := demoDraft()
	d.Settings.Audience = quiz.Audience{Type: quiz.AudienceCohort, Value: "control"}
	cohortQuiz := f.publish(t, d)
	if _, err := f.s.StartQuizAttempt(f.ctx, f.other, cohortQuiz, now); !errors.Is(err, ErrQuizUnavailable) {
		t.Fatalf("other cohort started: %v", err)
	}
	g := demoDraft()
	g.Settings.Audience = quiz.Audience{Type: quiz.AudienceGroup, Value: "qa-group"}
	groupQuiz := f.publish(t, g)
	if _, err := f.s.StartQuizAttempt(f.ctx, f.other, groupQuiz, now); !errors.Is(err, ErrQuizUnavailable) {
		t.Fatalf("non-member started: %v", err)
	}
	var email string
	_ = f.s.pool.QueryRow(f.ctx, `SELECT email FROM users WHERE id=$1`, f.other).Scan(&email)
	if missing, err := f.s.SetQuizGroupMembers(f.ctx, "qa-group", []string{email, "nobody@example.com"}); err != nil || len(missing) != 1 {
		t.Fatalf("set members: %v %v", missing, err)
	}
	if _, err := f.s.StartQuizAttempt(f.ctx, f.other, groupQuiz, now); err != nil {
		t.Fatalf("member could not start: %v", err)
	}

	r := demoDraft()
	r.Kind = quiz.KindFollowUp
	r.Settings.After = &quiz.RelativeWindow{Anchor: quiz.AnchorQuiz, QuizID: cohortQuiz, Days: 7}
	followUp := f.publish(t, r)
	cards, _ := f.s.ListQuizzesForLearner(f.ctx, f.learner, now)
	if c := findCard(cards, followUp); c == nil || c.Availability.State != quiz.StateUpcoming || c.Availability.Reason == "" {
		t.Fatalf("follow-up before anchor: %+v", c)
	}
	v, _ := f.s.StartQuizAttempt(f.ctx, f.learner, cohortQuiz, now)
	_ = f.s.SubmitQuizAttempt(f.ctx, f.learner, v.AttemptID, now)
	cards, _ = f.s.ListQuizzesForLearner(f.ctx, f.learner, now.Add(6*24*time.Hour))
	if c := findCard(cards, followUp); c == nil || c.Availability.State != quiz.StateUpcoming || c.Availability.OpensAt == nil {
		t.Fatalf("follow-up on day 6: %+v", c)
	}
	if _, err := f.s.StartQuizAttempt(f.ctx, f.learner, followUp, now.Add(6*24*time.Hour)); !errors.Is(err, ErrQuizUnavailable) {
		t.Fatalf("started follow-up early: %v", err)
	}
	if _, err := f.s.StartQuizAttempt(f.ctx, f.learner, followUp, now.Add(7*24*time.Hour+time.Minute)); err != nil {
		t.Fatalf("day 7 start: %v", err)
	}
}

func TestQuizEditingLocksQuestionsOnceAttemptsExist(t *testing.T) {
	f := newQuizFixture(t)
	id := f.publish(t, demoDraft())
	rec, err := f.s.GetQuiz(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	edited := rec.Draft
	edited.Questions[0].Prompt = "Changed before attempts"
	if err := f.s.UpdateQuiz(f.ctx, id, edited); err != nil {
		t.Fatalf("edit without attempts: %v", err)
	}
	if _, err := f.s.StartQuizAttempt(f.ctx, f.learner, id, time.Now()); err != nil {
		t.Fatal(err)
	}
	rec, _ = f.s.GetQuiz(f.ctx, id)
	settingsOnly := rec.Draft
	settingsOnly.Settings.MaxAttempts = 3
	settingsOnly.Title = "Renamed"
	if err := f.s.UpdateQuiz(f.ctx, id, settingsOnly); err != nil {
		t.Fatalf("settings edit after attempts: %v", err)
	}
	q := settingsOnly
	q.Questions = append([]quiz.Question(nil), q.Questions...)
	q.Questions[0].Options = append([]quiz.Option(nil), q.Questions[0].Options...)
	q.Questions[0].Options[0].Correct, q.Questions[0].Options[1].Correct = true, false
	if err := f.s.UpdateQuiz(f.ctx, id, q); !errors.Is(err, ErrQuizLocked) {
		t.Fatalf("question edit after attempts: %v", err)
	}
	dup, err := f.s.DuplicateQuiz(f.ctx, f.admin, id)
	if err != nil {
		t.Fatal(err)
	}
	copyRec, _ := f.s.GetQuiz(f.ctx, dup)
	if copyRec.Status != quiz.StatusDraft || !strings.HasPrefix(copyRec.Title, "Copy of") || len(copyRec.Questions) != 3 {
		t.Fatalf("duplicate: %+v", copyRec)
	}
	if err := f.s.DeleteQuiz(f.ctx, id); !errors.Is(err, ErrQuizLocked) {
		t.Fatalf("deleted a quiz with attempts: %v", err)
	}
	if err := f.s.DeleteQuiz(f.ctx, dup); err != nil {
		t.Fatalf("delete draft: %v", err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func findCard(cards []LearnerQuizCard, id string) *LearnerQuizCard {
	for i := range cards {
		if cards[i].ID == id {
			return &cards[i]
		}
	}
	return nil
}

func TestQuizCloseAndExpiredDeadlinesFinaliseAttemptsForAdmins(t *testing.T) {
	f := newQuizFixture(t)
	d := demoDraft()
	d.Settings.TimeLimitSeconds = 60
	d.Settings.MaxAttempts = 0
	id := f.publish(t, d)
	start := time.Now().Add(-10 * time.Minute)
	if _, err := f.s.StartQuizAttempt(f.ctx, f.learner, id, start); err != nil {
		t.Fatal(err)
	}
	// The learner never returns; the admin view must still see a finished attempt.
	list, err := f.s.ListQuizAttempts(f.ctx, id)
	if err != nil || len(list) != 1 || list[0].SubmittedAt == nil || *list[0].SubmitReason != "time_limit" {
		t.Fatalf("expired attempt not finalised for admin: %+v %v", list, err)
	}
	if _, err := f.s.StartQuizAttempt(f.ctx, f.other, id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SetQuizStatus(f.ctx, id, quiz.StatusClosed); err != nil {
		t.Fatal(err)
	}
	list, _ = f.s.ListQuizAttempts(f.ctx, id)
	for _, a := range list {
		if a.SubmittedAt == nil {
			t.Fatalf("closing left an open attempt: %+v", a)
		}
	}
	rows, _ := f.s.QuizExportRows(f.ctx, id)
	if len(rows) != 6 {
		t.Fatalf("export should include both finalised attempts: %d rows", len(rows))
	}
}
