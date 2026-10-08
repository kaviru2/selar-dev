// quiz.go — persistence for the quiz system (issue #86). Every rule that
// protects assessment integrity (audience, windows, attempts, deadlines,
// no-going-back, feedback policy) is enforced here or in internal/quiz, never
// only in the console.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/quiz"
)

var (
	ErrQuizNotFound        = errors.New("quiz not found")
	ErrQuizUnavailable     = errors.New("this quiz is not available to you right now")
	ErrQuizLocked          = errors.New("questions cannot change after learners have started attempts; duplicate the quiz instead")
	ErrAttemptNotFound     = errors.New("attempt not found")
	ErrAttemptSubmitted    = errors.New("this attempt has already been submitted")
	ErrAttemptNotSubmitted = errors.New("this attempt has not been submitted yet")
	ErrInvalidAnswer       = errors.New("invalid answer")
	ErrNoGoingBack         = errors.New("this quiz does not allow returning to or skipping ahead of the current question")
	ErrInvalidGrade        = errors.New("invalid grade")
)

const maxAnswerChars = 20000

// ---------- records ----------

type QuizRecord struct {
	ID          string      `json:"id"`
	Status      quiz.Status `json:"status"`
	Version     int         `json:"version"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	PublishedAt *time.Time  `json:"published_at,omitempty"`
	ClosedAt    *time.Time  `json:"closed_at,omitempty"`
	Attempts    int         `json:"attempts"`
	quiz.Draft
}

func (r QuizRecord) rules() quiz.Quiz {
	return quiz.Quiz{ID: r.ID, Status: r.Status, Kind: r.Kind, Settings: r.Settings}
}

type AdminQuizSummary struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Kind          quiz.Kind     `json:"kind"`
	Status        quiz.Status   `json:"status"`
	QuestionCount int           `json:"question_count"`
	Attempts      int           `json:"attempts"`
	Submitted     int           `json:"submitted"`
	PendingReview int           `json:"pending_review"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Settings      quiz.Settings `json:"settings"`
}

// ---------- admin: authoring ----------

func (s *Store) CreateQuiz(ctx context.Context, adminID string, d quiz.Draft) (string, error) {
	if err := d.Normalise(); err != nil {
		return "", err
	}
	settings, _ := json.Marshal(d.Settings)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var id string
	var creator any
	if adminID != "" {
		creator = adminID
	}
	if err := tx.QueryRow(ctx, `INSERT INTO quizzes(title, description, kind, settings, created_by)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`, d.Title, d.Description, d.Kind, settings, creator).Scan(&id); err != nil {
		return "", err
	}
	if err := insertQuestions(ctx, tx, id, d.Questions); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func insertQuestions(ctx context.Context, tx pgx.Tx, quizID string, qs []quiz.Question) error {
	for i, q := range qs {
		options, _ := json.Marshal(nonNilOptions(q.Options))
		accepted := q.AcceptedAnswers
		if accepted == nil {
			accepted = []string{}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO quiz_questions(quiz_id, position, type, prompt, cue, options, accepted_answers,
			points, rubric, explanation, source_document, source_url) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			quizID, i, q.Type, q.Prompt, q.Cue, options, accepted, q.Points, q.Rubric, q.Explanation, q.SourceDocument, q.SourceURL); err != nil {
			return err
		}
	}
	return nil
}

func nonNilOptions(o []quiz.Option) []quiz.Option {
	if o == nil {
		return []quiz.Option{}
	}
	return o
}

func (s *Store) GetQuiz(ctx context.Context, id string) (*QuizRecord, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrQuizNotFound
	}
	r := &QuizRecord{ID: id}
	var settings []byte
	err := s.pool.QueryRow(ctx, `SELECT title, description, kind, status, settings, version, created_at, updated_at,
		published_at, closed_at, (SELECT count(*) FROM quiz_attempts WHERE quiz_id = q.id)
		FROM quizzes q WHERE id = $1`, id).Scan(&r.Title, &r.Description, &r.Kind, &r.Status, &settings, &r.Version,
		&r.CreatedAt, &r.UpdatedAt, &r.PublishedAt, &r.ClosedAt, &r.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrQuizNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(settings, &r.Settings)
	r.Questions, err = s.quizQuestions(ctx, id)
	return r, err
}

func (s *Store) quizQuestions(ctx context.Context, quizID string) ([]quiz.Question, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, position, type, prompt, cue, options, accepted_answers, points::float8,
		rubric, explanation, source_document, source_url FROM quiz_questions WHERE quiz_id = $1 ORDER BY position`, quizID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []quiz.Question
	for rows.Next() {
		var q quiz.Question
		var options []byte
		if err := rows.Scan(&q.ID, &q.Position, &q.Type, &q.Prompt, &q.Cue, &options, &q.AcceptedAnswers, &q.Points,
			&q.Rubric, &q.Explanation, &q.SourceDocument, &q.SourceURL); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(options, &q.Options)
		if len(q.AcceptedAnswers) == 0 {
			q.AcceptedAnswers = nil
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// comparable strips database identity so two question lists can be compared by content.
func comparableQuestions(qs []quiz.Question) []quiz.Question {
	out := make([]quiz.Question, len(qs))
	for i, q := range qs {
		q.ID, q.Position = "", i
		if len(q.AcceptedAnswers) == 0 {
			q.AcceptedAnswers = nil
		}
		if len(q.Options) == 0 {
			q.Options = nil
		}
		out[i] = q
	}
	return out
}

// UpdateQuiz saves an edited draft. Settings and wording of the quiz itself can
// always change; questions are frozen once any attempt exists so every learner
// answers the same items with the same key.
func (s *Store) UpdateQuiz(ctx context.Context, id string, d quiz.Draft) error {
	if err := d.Normalise(); err != nil {
		return err
	}
	current, err := s.GetQuiz(ctx, id)
	if err != nil {
		return err
	}
	questionsChanged := !reflect.DeepEqual(comparableQuestions(current.Questions), comparableQuestions(d.Questions))
	if questionsChanged && current.Attempts > 0 {
		return ErrQuizLocked
	}
	settings, _ := json.Marshal(d.Settings)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	bump := 0
	if questionsChanged {
		bump = 1
		// Re-check under the transaction: an attempt may have started meanwhile.
		var attempts int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM quiz_attempts WHERE quiz_id=$1`, id).Scan(&attempts); err != nil {
			return err
		}
		if attempts > 0 {
			return ErrQuizLocked
		}
		if _, err := tx.Exec(ctx, `DELETE FROM quiz_questions WHERE quiz_id=$1`, id); err != nil {
			return err
		}
		if err := insertQuestions(ctx, tx, id, d.Questions); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE quizzes SET title=$2, description=$3, kind=$4, settings=$5, version=version+$6,
		updated_at=now() WHERE id=$1`, id, d.Title, d.Description, d.Kind, settings, bump); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SetQuizStatus(ctx context.Context, id string, status quiz.Status) error {
	switch status {
	case quiz.StatusDraft, quiz.StatusPublished, quiz.StatusClosed:
	default:
		return fmt.Errorf("unknown status %q", status)
	}
	if _, err := uuid.Parse(id); err != nil {
		return ErrQuizNotFound
	}
	tag, err := s.pool.Exec(ctx, `UPDATE quizzes SET status=$2, updated_at=now(),
		published_at = CASE WHEN $2='published' THEN COALESCE(published_at, now()) ELSE published_at END,
		closed_at = CASE WHEN $2='closed' THEN now() ELSE NULL END
		WHERE id=$1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrQuizNotFound
	}
	if status == quiz.StatusClosed {
		return s.expireQuizAttempts(ctx, id, time.Now())
	}
	return nil
}

func (s *Store) DuplicateQuiz(ctx context.Context, adminID, id string) (string, error) {
	r, err := s.GetQuiz(ctx, id)
	if err != nil {
		return "", err
	}
	d := r.Draft
	d.Title = "Copy of " + d.Title
	for i := range d.Questions {
		d.Questions[i].ID = ""
	}
	return s.CreateQuiz(ctx, adminID, d)
}

func (s *Store) DeleteQuiz(ctx context.Context, id string) error {
	r, err := s.GetQuiz(ctx, id)
	if err != nil {
		return err
	}
	if r.Attempts > 0 {
		return fmt.Errorf("%w (close it instead of deleting it)", ErrQuizLocked)
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM quizzes WHERE id=$1 AND NOT EXISTS (SELECT 1 FROM quiz_attempts WHERE quiz_id=$1)`, id)
	return err
}

func (s *Store) ListQuizzesAdmin(ctx context.Context) ([]AdminQuizSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT q.id, q.title, q.kind, q.status, q.settings, q.updated_at,
		(SELECT count(*) FROM quiz_questions WHERE quiz_id=q.id),
		(SELECT count(*) FROM quiz_attempts WHERE quiz_id=q.id),
		(SELECT count(*) FROM quiz_attempts WHERE quiz_id=q.id AND submitted_at IS NOT NULL),
		(SELECT COALESCE(sum(pending_review),0) FROM quiz_attempts WHERE quiz_id=q.id AND submitted_at IS NOT NULL)
		FROM quizzes q ORDER BY q.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminQuizSummary{}
	for rows.Next() {
		var a AdminQuizSummary
		var settings []byte
		if err := rows.Scan(&a.ID, &a.Title, &a.Kind, &a.Status, &settings, &a.UpdatedAt, &a.QuestionCount, &a.Attempts, &a.Submitted, &a.PendingReview); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(settings, &a.Settings)
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetQuizGroupMembers replaces the members of an audience label, matching
// users by email. Emails without an account are returned, not invented.
func (s *Store) SetQuizGroupMembers(ctx context.Context, label string, emails []string) ([]string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, errors.New("group label is required")
	}
	clean := []string{}
	for _, e := range emails {
		if e = strings.TrimSpace(e); e != "" {
			clean = append(clean, e)
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `DELETE FROM quiz_group_members WHERE label=$1`, label); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO quiz_group_members(label, user_id)
		SELECT $1, id FROM users WHERE lower(email) = ANY(SELECT lower(x) FROM unnest($2::text[]) x) ON CONFLICT DO NOTHING`, label, clean); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT x FROM unnest($1::text[]) x WHERE NOT EXISTS (SELECT 1 FROM users WHERE lower(email)=lower(x))`, clean)
	if err != nil {
		return nil, err
	}
	missing := []string{}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			rows.Close()
			return nil, err
		}
		missing = append(missing, m)
	}
	rows.Close()
	return missing, tx.Commit(ctx)
}

type QuizGroup struct {
	Label  string   `json:"label"`
	Emails []string `json:"emails"`
}

func (s *Store) ListQuizGroups(ctx context.Context) ([]QuizGroup, error) {
	rows, err := s.pool.Query(ctx, `SELECT g.label, array_agg(u.email ORDER BY u.email) FROM quiz_group_members g
		JOIN users u ON u.id=g.user_id GROUP BY g.label ORDER BY g.label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QuizGroup{}
	for rows.Next() {
		var g QuizGroup
		if err := rows.Scan(&g.Label, &g.Emails); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ---------- learner context ----------

func (s *Store) learnerContext(ctx context.Context, userID string) (quiz.Learner, error) {
	l := quiz.Learner{DocumentReadAt: map[string]time.Time{}, QuizSubmittedAt: map[string]time.Time{}}
	if err := s.pool.QueryRow(ctx, `SELECT cohort,
		COALESCE((SELECT array_agg(label) FROM quiz_group_members WHERE user_id=$1), '{}'),
		(SELECT min(started_at) FROM reading_sessions WHERE user_id=$1)
		FROM users WHERE id=$1`, userID).Scan(&l.Cohort, &l.Groups, &l.FirstReadingAt); err != nil {
		return l, err
	}
	rows, err := s.pool.Query(ctx, `SELECT d.content_hash, d.title, min(r.started_at) FROM reading_sessions r
		JOIN documents d ON d.id=r.document_id WHERE r.user_id=$1 GROUP BY d.content_hash, d.title`, userID)
	if err != nil {
		return l, err
	}
	for rows.Next() {
		var hash, title string
		var at time.Time
		if err := rows.Scan(&hash, &title, &at); err != nil {
			rows.Close()
			return l, err
		}
		for _, key := range []string{hash, title} {
			if prev, ok := l.DocumentReadAt[key]; key != "" && (!ok || at.Before(prev)) {
				l.DocumentReadAt[key] = at
			}
		}
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT quiz_id, min(submitted_at) FROM quiz_attempts WHERE user_id=$1 AND submitted_at IS NOT NULL GROUP BY quiz_id`, userID)
	if err != nil {
		return l, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return l, err
		}
		l.QuizSubmittedAt[id] = at
	}
	return l, rows.Err()
}

type attemptStats struct {
	used          int
	openID        string
	lastID        string
	lastSubmitted *time.Time
	lastScore     *float64
	lastMax       float64
	lastPending   int
}

func (s *Store) attemptStatsByQuiz(ctx context.Context, userID string) (map[string]*attemptStats, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, quiz_id, submitted_at, score::float8, max_points::float8, pending_review
		FROM quiz_attempts WHERE user_id=$1 ORDER BY attempt_number`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*attemptStats{}
	for rows.Next() {
		var id, quizID string
		var submitted *time.Time
		var score *float64
		var max float64
		var pending int
		if err := rows.Scan(&id, &quizID, &submitted, &score, &max, &pending); err != nil {
			return nil, err
		}
		st := out[quizID]
		if st == nil {
			st = &attemptStats{}
			out[quizID] = st
		}
		st.used++
		st.lastID = id
		if submitted == nil {
			st.openID = id
		} else {
			st.lastSubmitted, st.lastScore, st.lastMax, st.lastPending = submitted, score, max, pending
		}
	}
	return out, rows.Err()
}

// ---------- learner: listing ----------

type LastAttempt struct {
	ID          string     `json:"id"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	// Score fields are only present when the feedback policy allows.
	Score         *float64 `json:"score,omitempty"`
	MaxPoints     *float64 `json:"max_points,omitempty"`
	PendingReview *int     `json:"pending_review,omitempty"`
	ResultVisible bool     `json:"result_visible"`
}

type LearnerQuizCard struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	Kind             quiz.Kind         `json:"kind"`
	QuestionCount    int               `json:"question_count"`
	TimeLimitSeconds int               `json:"time_limit_seconds,omitempty"`
	NoGoingBack      bool              `json:"no_going_back"`
	Availability     quiz.Availability `json:"availability"`
	OpenAttemptID    string            `json:"open_attempt_id,omitempty"`
	LastAttempt      *LastAttempt      `json:"last_attempt,omitempty"`
}

func (s *Store) ListQuizzesForLearner(ctx context.Context, userID string, now time.Time) ([]LearnerQuizCard, error) {
	if err := s.expireOpenAttempts(ctx, userID, now); err != nil {
		return nil, err
	}
	learner, err := s.learnerContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	stats, err := s.attemptStatsByQuiz(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT q.id, q.title, q.description, q.kind, q.status, q.settings,
		(SELECT count(*) FROM quiz_questions WHERE quiz_id=q.id)
		FROM quizzes q WHERE q.status <> 'draft' ORDER BY COALESCE(q.published_at, q.created_at)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LearnerQuizCard{}
	for rows.Next() {
		var c LearnerQuizCard
		var status quiz.Status
		var raw []byte
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Kind, &status, &raw, &c.QuestionCount); err != nil {
			return nil, err
		}
		var settings quiz.Settings
		_ = json.Unmarshal(raw, &settings)
		st := stats[c.ID]
		if st == nil {
			st = &attemptStats{}
		}
		rules := quiz.Quiz{ID: c.ID, Status: status, Kind: c.Kind, Settings: settings}
		c.Availability = quiz.Evaluate(rules, learner, quiz.Attempts{Used: st.used, InProgress: st.openID != ""}, now)
		if c.Availability.State == quiz.StateHidden {
			continue
		}
		c.TimeLimitSeconds, c.NoGoingBack, c.OpenAttemptID = settings.TimeLimitSeconds, settings.NoGoingBack, st.openID
		if st.lastSubmitted != nil {
			last := &LastAttempt{ID: st.lastID, SubmittedAt: st.lastSubmitted}
			if st.openID != "" {
				last.ID = ""
			}
			if quiz.FeedbackVisibleWithClose(rules, true, now, c.Availability.ClosesAt) {
				max, pending := st.lastMax, st.lastPending
				last.Score, last.MaxPoints, last.PendingReview, last.ResultVisible = st.lastScore, &max, &pending, true
			}
			c.LastAttempt = last
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- learner: attempts ----------

type LearnerOption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// LearnerQuestion is the question as a learner may see it during an attempt:
// no correct flags, accepted answers, rubric, explanation or source link.
type LearnerQuestion struct {
	ID      string            `json:"id"`
	Index   int               `json:"index"`
	Type    quiz.QuestionType `json:"type"`
	Prompt  string            `json:"prompt"`
	Cue     string            `json:"cue,omitempty"`
	Points  float64           `json:"points"`
	Options []LearnerOption   `json:"options,omitempty"`
}

type SavedAnswer struct {
	Selected []string `json:"selected"`
	Text     string   `json:"text"`
}

type AttemptView struct {
	AttemptID       string                 `json:"attempt_id"`
	QuizID          string                 `json:"quiz_id"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	Kind            quiz.Kind              `json:"kind"`
	StartedAt       time.Time              `json:"started_at"`
	DeadlineAt      *time.Time             `json:"deadline_at,omitempty"`
	ServerNow       time.Time              `json:"server_now"`
	NoGoingBack     bool                   `json:"no_going_back"`
	Total           int                    `json:"total"`
	CurrentPosition int                    `json:"current_position"`
	Questions       []LearnerQuestion      `json:"questions"`
	Answers         map[string]SavedAnswer `json:"answers"`
}

type attemptRow struct {
	id, quizID, userID string
	startedAt          time.Time
	deadline           *time.Time
	submitted          *time.Time
	order              []string
	optionOrder        map[string][]string
	position           int
}

func scanAttempt(row pgx.Row) (*attemptRow, error) {
	a := &attemptRow{}
	var optionOrder []byte
	err := row.Scan(&a.id, &a.quizID, &a.userID, &a.startedAt, &a.deadline, &a.submitted, &a.order, &optionOrder, &a.position)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttemptNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(optionOrder, &a.optionOrder)
	return a, nil
}

const attemptCols = `id, quiz_id, user_id, started_at, deadline_at, submitted_at, question_order, option_order, current_position`

func (s *Store) StartQuizAttempt(ctx context.Context, userID, quizID string, now time.Time) (*AttemptView, error) {
	if err := s.expireOpenAttempts(ctx, userID, now); err != nil {
		return nil, err
	}
	rec, err := s.GetQuiz(ctx, quizID)
	if errors.Is(err, ErrQuizNotFound) {
		return nil, ErrQuizUnavailable
	}
	if err != nil {
		return nil, err
	}
	learner, err := s.learnerContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	stats, err := s.attemptStatsByQuiz(ctx, userID)
	if err != nil {
		return nil, err
	}
	st := stats[quizID]
	if st == nil {
		st = &attemptStats{}
	}
	avail := quiz.Evaluate(rec.rules(), learner, quiz.Attempts{Used: st.used, InProgress: st.openID != ""}, now)
	switch avail.State {
	case quiz.StateInProgress:
		return s.GetAttemptView(ctx, userID, st.openID, now)
	case quiz.StateAvailable:
	default:
		return nil, ErrQuizUnavailable
	}
	attemptID := uuid.NewString()
	ids := make([]string, len(rec.Questions))
	optionOrder := map[string][]string{}
	for i, q := range rec.Questions {
		ids[i] = q.ID
		if rec.Settings.ShuffleOptions && q.Type != quiz.TypeTrueFalse && len(q.Options) > 1 {
			var opts []string
			for _, o := range q.Options {
				opts = append(opts, o.ID)
			}
			optionOrder[q.ID] = quiz.ShuffledIDs(opts, attemptID+q.ID)
		}
	}
	if rec.Settings.ShuffleQuestions {
		ids = quiz.ShuffledIDs(ids, attemptID)
	}
	deadline := quiz.Deadline(rec.Settings.TimeLimitSeconds, now, avail.ClosesAt)
	maxPoints := 0.0
	for _, q := range rec.Questions {
		maxPoints += q.Points
	}
	orderJSON, _ := json.Marshal(optionOrder)
	_, err = s.pool.Exec(ctx, `INSERT INTO quiz_attempts(id, quiz_id, user_id, attempt_number, quiz_version, started_at,
		deadline_at, question_order, option_order, max_points)
		VALUES ($1,$2,$3,(SELECT COALESCE(max(attempt_number),0)+1 FROM quiz_attempts WHERE quiz_id=$2 AND user_id=$3),
		$4,$5,$6,$7,$8,$9)`, attemptID, quizID, userID, rec.Version, now, deadline, ids, orderJSON, maxPoints)
	if err != nil {
		// A concurrent start won the unique index: resume that attempt instead.
		if strings.Contains(err.Error(), "idx_quiz_attempts_one_open") || strings.Contains(err.Error(), "quiz_attempts_quiz_id_user_id_attempt_number_key") {
			return s.StartQuizAttempt(ctx, userID, quizID, now)
		}
		return nil, err
	}
	return s.GetAttemptView(ctx, userID, attemptID, now)
}

func (s *Store) ownedAttempt(ctx context.Context, userID, attemptID string) (*attemptRow, error) {
	if _, err := uuid.Parse(attemptID); err != nil {
		return nil, ErrAttemptNotFound
	}
	return scanAttempt(s.pool.QueryRow(ctx, `SELECT `+attemptCols+` FROM quiz_attempts WHERE id=$1 AND user_id=$2`, attemptID, userID))
}

func (s *Store) GetAttemptView(ctx context.Context, userID, attemptID string, now time.Time) (*AttemptView, error) {
	if err := s.expireOpenAttempts(ctx, userID, now); err != nil {
		return nil, err
	}
	a, err := s.ownedAttempt(ctx, userID, attemptID)
	if err != nil {
		return nil, err
	}
	if a.submitted != nil {
		return nil, ErrAttemptSubmitted
	}
	rec, err := s.GetQuiz(ctx, a.quizID)
	if err != nil {
		return nil, err
	}
	byID := map[string]quiz.Question{}
	for _, q := range rec.Questions {
		byID[q.ID] = q
	}
	v := &AttemptView{AttemptID: a.id, QuizID: rec.ID, Title: rec.Title, Description: rec.Description, Kind: rec.Kind,
		StartedAt: a.startedAt, DeadlineAt: a.deadline, ServerNow: now, NoGoingBack: rec.Settings.NoGoingBack,
		Total: len(a.order), CurrentPosition: a.position, Answers: map[string]SavedAnswer{}}
	for i, qid := range a.order {
		if rec.Settings.NoGoingBack && i != a.position {
			continue
		}
		q := byID[qid]
		lq := LearnerQuestion{ID: q.ID, Index: i, Type: q.Type, Prompt: q.Prompt, Cue: q.Cue, Points: q.Points}
		optByID := map[string]quiz.Option{}
		for _, o := range q.Options {
			optByID[o.ID] = o
		}
		order := a.optionOrder[q.ID]
		if len(order) == 0 {
			for _, o := range q.Options {
				order = append(order, o.ID)
			}
		}
		for _, id := range order {
			lq.Options = append(lq.Options, LearnerOption{ID: id, Text: optByID[id].Text})
		}
		v.Questions = append(v.Questions, lq)
	}
	rows, err := s.pool.Query(ctx, `SELECT question_id, selected, answer_text FROM quiz_answers WHERE attempt_id=$1`, a.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	visible := map[string]bool{}
	for _, q := range v.Questions {
		visible[q.ID] = true
	}
	for rows.Next() {
		var qid string
		var ans SavedAnswer
		if err := rows.Scan(&qid, &ans.Selected, &ans.Text); err != nil {
			return nil, err
		}
		if visible[qid] {
			v.Answers[qid] = ans
		}
	}
	return v, rows.Err()
}

type SaveAnswerInput struct {
	QuestionID       string   `json:"question_id"`
	Selected         []string `json:"selected"`
	Text             string   `json:"text"`
	TimeOnQuestionMs int64    `json:"time_on_question_ms"`
}

func (s *Store) SaveQuizAnswer(ctx context.Context, userID, attemptID string, in SaveAnswerInput, now time.Time) error {
	if err := s.expireOpenAttempts(ctx, userID, now); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := uuid.Parse(attemptID); err != nil {
		return ErrAttemptNotFound
	}
	a, err := scanAttempt(tx.QueryRow(ctx, `SELECT `+attemptCols+` FROM quiz_attempts WHERE id=$1 AND user_id=$2 FOR UPDATE`, attemptID, userID))
	if err != nil {
		return err
	}
	if a.submitted != nil {
		return ErrAttemptSubmitted
	}
	idx := -1
	for i, id := range a.order {
		if id == in.QuestionID {
			idx = i
		}
	}
	if idx < 0 {
		return fmt.Errorf("%w: question is not part of this attempt", ErrInvalidAnswer)
	}
	var noGoingBack bool
	var qType quiz.QuestionType
	var optionsJSON []byte
	if err := tx.QueryRow(ctx, `SELECT COALESCE((q.settings->>'no_going_back')::bool, false), qq.type, qq.options
		FROM quizzes q JOIN quiz_questions qq ON qq.quiz_id=q.id WHERE qq.id=$1`, in.QuestionID).Scan(&noGoingBack, &qType, &optionsJSON); err != nil {
		return err
	}
	if noGoingBack && idx != a.position {
		return ErrNoGoingBack
	}
	selected := []string{}
	text := ""
	if qType.IsChoice() {
		var opts []quiz.Option
		_ = json.Unmarshal(optionsJSON, &opts)
		valid := map[string]bool{}
		for _, o := range opts {
			valid[o.ID] = true
		}
		seen := map[string]bool{}
		for _, id := range in.Selected {
			if !valid[id] {
				return fmt.Errorf("%w: unknown option", ErrInvalidAnswer)
			}
			if !seen[id] {
				seen[id] = true
				selected = append(selected, id)
			}
		}
		if qType != quiz.TypeMultipleChoice && len(selected) > 1 {
			return fmt.Errorf("%w: choose one option", ErrInvalidAnswer)
		}
	} else {
		if len([]rune(in.Text)) > maxAnswerChars {
			return fmt.Errorf("%w: answers are limited to %d characters", ErrInvalidAnswer, maxAnswerChars)
		}
		text = in.Text
	}
	// Time on question is reported by the client; cap it by the wall-clock
	// time since the attempt started so it can never exceed what is possible.
	wall := now.Sub(a.startedAt).Milliseconds()
	ms := in.TimeOnQuestionMs
	if ms < 0 {
		ms = 0
	}
	if ms > wall {
		ms = wall
	}
	if ms < 0 {
		ms = 0
	}
	if _, err := tx.Exec(ctx, `INSERT INTO quiz_answers(attempt_id, question_id, selected, answer_text, first_seen_at,
		answered_at, updated_at, time_on_question_ms) VALUES ($1,$2,$3,$4,$5,$5,$5,$6)
		ON CONFLICT (attempt_id, question_id) DO UPDATE SET selected=EXCLUDED.selected, answer_text=EXCLUDED.answer_text,
		answered_at=EXCLUDED.answered_at, updated_at=EXCLUDED.updated_at, revisions=quiz_answers.revisions+1,
		time_on_question_ms=GREATEST(quiz_answers.time_on_question_ms, EXCLUDED.time_on_question_ms)`,
		a.id, in.QuestionID, selected, text, now, ms); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AdvanceQuizAttempt moves a no-going-back attempt to the next question.
func (s *Store) AdvanceQuizAttempt(ctx context.Context, userID, attemptID string, now time.Time) (*AttemptView, error) {
	if err := s.expireOpenAttempts(ctx, userID, now); err != nil {
		return nil, err
	}
	a, err := s.ownedAttempt(ctx, userID, attemptID)
	if err != nil {
		return nil, err
	}
	if a.submitted != nil {
		return nil, ErrAttemptSubmitted
	}
	if _, err := s.pool.Exec(ctx, `UPDATE quiz_attempts SET current_position = LEAST(current_position+1, cardinality(question_order)-1)
		WHERE id=$1 AND submitted_at IS NULL AND current_position=$2`, a.id, a.position); err != nil {
		return nil, err
	}
	return s.GetAttemptView(ctx, userID, attemptID, now)
}

func (s *Store) SubmitQuizAttempt(ctx context.Context, userID, attemptID string, now time.Time) error {
	if err := s.expireOpenAttempts(ctx, userID, now); err != nil {
		return err
	}
	a, err := s.ownedAttempt(ctx, userID, attemptID)
	if err != nil {
		return err
	}
	if a.submitted != nil {
		return nil
	}
	return s.finaliseAttempt(ctx, a.id, "learner", now)
}

// expireOpenAttempts submits any of the user's attempts whose deadline has
// passed or whose quiz was closed, so late answers can never be saved.
func (s *Store) expireOpenAttempts(ctx context.Context, userID string, now time.Time) error {
	return s.expireWhere(ctx, `a.user_id=$1`, userID, now)
}

// expireQuizAttempts does the same for every learner of one quiz, so admin
// views and exports never show attempts that should already be final.
func (s *Store) expireQuizAttempts(ctx context.Context, quizID string, now time.Time) error {
	return s.expireWhere(ctx, `a.quiz_id=$1`, quizID, now)
}

func (s *Store) expireWhere(ctx context.Context, scope, arg string, now time.Time) error {
	rows, err := s.pool.Query(ctx, `SELECT a.id, a.deadline_at, q.status FROM quiz_attempts a JOIN quizzes q ON q.id=a.quiz_id
		WHERE `+scope+` AND a.submitted_at IS NULL AND ((a.deadline_at IS NOT NULL AND a.deadline_at <= $2) OR q.status='closed')`, arg, now)
	if err != nil {
		return err
	}
	type exp struct {
		id       string
		deadline *time.Time
		status   string
	}
	var expired []exp
	for rows.Next() {
		var e exp
		if err := rows.Scan(&e.id, &e.deadline, &e.status); err != nil {
			rows.Close()
			return err
		}
		expired = append(expired, e)
	}
	rows.Close()
	for _, e := range expired {
		reason, at := "closed", now
		if e.deadline != nil && !e.deadline.After(now) {
			reason, at = "time_limit", *e.deadline
		}
		if err := s.finaliseAttempt(ctx, e.id, reason, at); err != nil {
			return err
		}
	}
	return nil
}

// finaliseAttempt grades every question (blank answers included) and marks
// the attempt submitted, atomically and at most once.
func (s *Store) finaliseAttempt(ctx context.Context, attemptID, reason string, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var submitted *time.Time
	var quizID string
	var order []string
	if err := tx.QueryRow(ctx, `SELECT submitted_at, quiz_id, question_order FROM quiz_attempts WHERE id=$1 FOR UPDATE`, attemptID).Scan(&submitted, &quizID, &order); err != nil {
		return err
	}
	if submitted != nil {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO quiz_answers(attempt_id, question_id, first_seen_at, updated_at)
		SELECT $1, qid, $2, $2 FROM unnest($3::uuid[]) qid ON CONFLICT DO NOTHING`, attemptID, at, order); err != nil {
		return err
	}
	questions := map[string]quiz.Question{}
	qrows, err := tx.Query(ctx, `SELECT id, type, options, accepted_answers, points::float8 FROM quiz_questions WHERE quiz_id=$1`, quizID)
	if err != nil {
		return err
	}
	for qrows.Next() {
		var q quiz.Question
		var opts []byte
		if err := qrows.Scan(&q.ID, &q.Type, &opts, &q.AcceptedAnswers, &q.Points); err != nil {
			qrows.Close()
			return err
		}
		_ = json.Unmarshal(opts, &q.Options)
		questions[q.ID] = q
	}
	qrows.Close()
	arows, err := tx.Query(ctx, `SELECT id, question_id, selected, answer_text FROM quiz_answers WHERE attempt_id=$1`, attemptID)
	if err != nil {
		return err
	}
	type graded struct {
		id string
		g  quiz.Graded
	}
	var results []graded
	for arows.Next() {
		var id, qid, text string
		var selected []string
		if err := arows.Scan(&id, &qid, &selected, &text); err != nil {
			arows.Close()
			return err
		}
		results = append(results, graded{id, quiz.Grade(questions[qid], quiz.Answer{Selected: selected, Text: text})})
	}
	arows.Close()
	for _, r := range results {
		if _, err := tx.Exec(ctx, `UPDATE quiz_answers SET auto_score=$2, is_correct=$3, needs_review=$4 WHERE id=$1`,
			r.id, r.g.Score, r.g.Correct, r.g.NeedsReview); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE quiz_attempts SET submitted_at=$2, submit_reason=$3 WHERE id=$1`, attemptID, at, reason); err != nil {
		return err
	}
	if err := recomputeTotals(ctx, tx, attemptID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func recomputeTotals(ctx context.Context, tx pgx.Tx, attemptID string) error {
	_, err := tx.Exec(ctx, `WITH t AS (
		SELECT COALESCE(sum(COALESCE(manual_score, auto_score)),0) AS pts,
		       count(*) FILTER (WHERE needs_review AND manual_score IS NULL) AS pending
		FROM quiz_answers WHERE attempt_id=$1)
		UPDATE quiz_attempts SET auto_points=t.pts, pending_review=t.pending,
		       score = CASE WHEN t.pending=0 THEN t.pts ELSE NULL END
		FROM t WHERE id=$1`, attemptID)
	return err
}

// ---------- learner: results ----------

type ResultOption struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Correct  bool   `json:"correct"`
	Selected bool   `json:"selected"`
}

type ResultQuestion struct {
	ID              string            `json:"id"`
	Index           int               `json:"index"`
	Type            quiz.QuestionType `json:"type"`
	Prompt          string            `json:"prompt"`
	Cue             string            `json:"cue,omitempty"`
	Options         []ResultOption    `json:"options,omitempty"`
	Text            string            `json:"text,omitempty"`
	AcceptedAnswers []string          `json:"accepted_answers,omitempty"`
	Points          float64           `json:"points"`
	Score           *float64          `json:"score,omitempty"`
	Correct         *bool             `json:"correct,omitempty"`
	PendingReview   bool              `json:"pending_review"`
	Explanation     string            `json:"explanation,omitempty"`
	GraderNote      string            `json:"grader_note,omitempty"`
	SourceDocument  string            `json:"source_document,omitempty"`
	SourceURL       string            `json:"source_url,omitempty"`
}

type AttemptResult struct {
	AttemptID         string              `json:"attempt_id"`
	QuizID            string              `json:"quiz_id"`
	Title             string              `json:"title"`
	SubmittedAt       time.Time           `json:"submitted_at"`
	SubmitReason      string              `json:"submit_reason"`
	FeedbackAvailable bool                `json:"feedback_available"`
	FeedbackPolicy    quiz.FeedbackPolicy `json:"feedback_policy"`
	Summary           *quiz.Summary       `json:"summary,omitempty"`
	Questions         []ResultQuestion    `json:"questions,omitempty"`
}

func (s *Store) QuizResult(ctx context.Context, userID, attemptID string, now time.Time) (*AttemptResult, error) {
	if err := s.expireOpenAttempts(ctx, userID, now); err != nil {
		return nil, err
	}
	a, err := s.ownedAttempt(ctx, userID, attemptID)
	if err != nil {
		return nil, err
	}
	if a.submitted == nil {
		return nil, ErrAttemptNotSubmitted
	}
	rec, err := s.GetQuiz(ctx, a.quizID)
	if err != nil {
		return nil, err
	}
	learner, err := s.learnerContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	_, closeAt, _, _ := quiz.Window(rec.Settings, learner)
	res := &AttemptResult{AttemptID: a.id, QuizID: rec.ID, Title: rec.Title, SubmittedAt: *a.submitted, FeedbackPolicy: rec.Settings.Feedback}
	var max, autoPts float64
	var pending int
	var score *float64
	if err := s.pool.QueryRow(ctx, `SELECT submit_reason, max_points::float8, auto_points::float8, pending_review, score::float8
		FROM quiz_attempts WHERE id=$1`, a.id).Scan(&res.SubmitReason, &max, &autoPts, &pending, &score); err != nil {
		return nil, err
	}
	res.FeedbackAvailable = quiz.FeedbackVisibleWithClose(rec.rules(), true, now, closeAt)
	if !res.FeedbackAvailable {
		return res, nil
	}
	sum := quiz.Summary{MaxPoints: max, AutoPoints: autoPts, PendingReview: pending, Score: score}
	if score != nil && max > 0 {
		pct := *score / max * 100
		sum.Percent = &pct
	}
	res.Summary = &sum
	byID := map[string]quiz.Question{}
	for _, q := range rec.Questions {
		byID[q.ID] = q
	}
	rows, err := s.pool.Query(ctx, `SELECT question_id, selected, answer_text, COALESCE(manual_score, auto_score)::float8,
		is_correct, needs_review AND manual_score IS NULL, grader_note FROM quiz_answers WHERE attempt_id=$1`, a.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	answers := map[string]ResultQuestion{}
	for rows.Next() {
		var qid string
		var selected []string
		var r ResultQuestion
		if err := rows.Scan(&qid, &selected, &r.Text, &r.Score, &r.Correct, &r.PendingReview, &r.GraderNote); err != nil {
			return nil, err
		}
		picked := map[string]bool{}
		for _, id := range selected {
			picked[id] = true
		}
		q := byID[qid]
		for _, o := range q.Options {
			r.Options = append(r.Options, ResultOption{ID: o.ID, Text: o.Text, Correct: o.Correct, Selected: picked[o.ID]})
		}
		answers[qid] = r
	}
	for i, qid := range a.order {
		q := byID[qid]
		r := answers[qid]
		r.ID, r.Index, r.Type, r.Prompt, r.Cue, r.Points = q.ID, i, q.Type, q.Prompt, q.Cue, q.Points
		r.AcceptedAnswers, r.Explanation, r.SourceDocument, r.SourceURL = q.AcceptedAnswers, q.Explanation, q.SourceDocument, q.SourceURL
		res.Questions = append(res.Questions, r)
	}
	return res, rows.Err()
}

// ---------- admin: results, grading, export ----------

type AdminAttemptSummary struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Email         string     `json:"email"`
	Cohort        string     `json:"cohort"`
	AttemptNumber int        `json:"attempt_number"`
	QuizVersion   int        `json:"quiz_version"`
	StartedAt     time.Time  `json:"started_at"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
	SubmitReason  *string    `json:"submit_reason,omitempty"`
	DurationMs    *int64     `json:"duration_ms,omitempty"`
	MaxPoints     float64    `json:"max_points"`
	AutoPoints    float64    `json:"auto_points"`
	PendingReview int        `json:"pending_review"`
	Score         *float64   `json:"score,omitempty"`
}

func (s *Store) ListQuizAttempts(ctx context.Context, quizID string) ([]AdminAttemptSummary, error) {
	if _, err := uuid.Parse(quizID); err != nil {
		return nil, ErrQuizNotFound
	}
	if err := s.expireQuizAttempts(ctx, quizID, time.Now()); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id, a.user_id, u.email, u.cohort, a.attempt_number, a.quiz_version, a.started_at,
		a.submitted_at, a.submit_reason, (extract(epoch FROM a.submitted_at - a.started_at)*1000)::bigint,
		a.max_points::float8, a.auto_points::float8, a.pending_review, a.score::float8
		FROM quiz_attempts a JOIN users u ON u.id=a.user_id WHERE a.quiz_id=$1 ORDER BY a.started_at`, quizID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminAttemptSummary{}
	for rows.Next() {
		var a AdminAttemptSummary
		if err := rows.Scan(&a.ID, &a.UserID, &a.Email, &a.Cohort, &a.AttemptNumber, &a.QuizVersion, &a.StartedAt, &a.SubmittedAt,
			&a.SubmitReason, &a.DurationMs, &a.MaxPoints, &a.AutoPoints, &a.PendingReview, &a.Score); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type AdminAnswer struct {
	ID               string            `json:"id"`
	QuestionID       string            `json:"question_id"`
	Index            int               `json:"index"`
	Type             quiz.QuestionType `json:"type"`
	Prompt           string            `json:"prompt"`
	Cue              string            `json:"cue,omitempty"`
	Rubric           string            `json:"rubric,omitempty"`
	AcceptedAnswers  []string          `json:"accepted_answers,omitempty"`
	Options          []quiz.Option     `json:"options,omitempty"`
	Points           float64           `json:"points"`
	Selected         []string          `json:"selected"`
	Text             string            `json:"text"`
	AutoScore        *float64          `json:"auto_score,omitempty"`
	ManualScore      *float64          `json:"manual_score,omitempty"`
	IsCorrect        *bool             `json:"is_correct,omitempty"`
	NeedsReview      bool              `json:"needs_review"`
	GraderNote       string            `json:"grader_note"`
	TimeOnQuestionMs int64             `json:"time_on_question_ms"`
	AnsweredAt       *time.Time        `json:"answered_at,omitempty"`
	Revisions        int               `json:"revisions"`
}

// AdminAttemptDetail deliberately omits learner identity and cohort so
// manual scoring can be done blind.
type AdminAttemptDetail struct {
	ID          string        `json:"id"`
	QuizID      string        `json:"quiz_id"`
	QuizTitle   string        `json:"quiz_title"`
	SubmittedAt *time.Time    `json:"submitted_at,omitempty"`
	Summary     quiz.Summary  `json:"summary"`
	Answers     []AdminAnswer `json:"answers"`
}

func (s *Store) GetAttemptForAdmin(ctx context.Context, attemptID string) (*AdminAttemptDetail, error) {
	if _, err := uuid.Parse(attemptID); err != nil {
		return nil, ErrAttemptNotFound
	}
	d := &AdminAttemptDetail{ID: attemptID}
	var order []string
	err := s.pool.QueryRow(ctx, `SELECT a.quiz_id, q.title, a.submitted_at, a.question_order, a.max_points::float8,
		a.auto_points::float8, a.pending_review, a.score::float8 FROM quiz_attempts a JOIN quizzes q ON q.id=a.quiz_id WHERE a.id=$1`,
		attemptID).Scan(&d.QuizID, &d.QuizTitle, &d.SubmittedAt, &order, &d.Summary.MaxPoints, &d.Summary.AutoPoints, &d.Summary.PendingReview, &d.Summary.Score)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttemptNotFound
	}
	if err != nil {
		return nil, err
	}
	position := map[string]int{}
	for i, id := range order {
		position[id] = i
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id, q.id, q.type, q.prompt, q.cue, q.rubric, q.accepted_answers, q.options, q.points::float8,
		a.selected, a.answer_text, a.auto_score::float8, a.manual_score::float8, a.is_correct, a.needs_review, a.grader_note,
		a.time_on_question_ms, a.answered_at, a.revisions
		FROM quiz_answers a JOIN quiz_questions q ON q.id=a.question_id WHERE a.attempt_id=$1`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var x AdminAnswer
		var opts []byte
		if err := rows.Scan(&x.ID, &x.QuestionID, &x.Type, &x.Prompt, &x.Cue, &x.Rubric, &x.AcceptedAnswers, &opts, &x.Points,
			&x.Selected, &x.Text, &x.AutoScore, &x.ManualScore, &x.IsCorrect, &x.NeedsReview, &x.GraderNote,
			&x.TimeOnQuestionMs, &x.AnsweredAt, &x.Revisions); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(opts, &x.Options)
		x.Index = position[x.QuestionID]
		d.Answers = append(d.Answers, x)
	}
	sortAnswers(d.Answers)
	return d, rows.Err()
}

func sortAnswers(a []AdminAnswer) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j].Index < a[j-1].Index; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// GradeQuizAnswer records (or, with score nil, clears) a manual score.
func (s *Store) GradeQuizAnswer(ctx context.Context, graderID, answerID string, score *float64, note string) error {
	if _, err := uuid.Parse(answerID); err != nil {
		return ErrAttemptNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var attemptID string
	var points float64
	var submitted *time.Time
	err = tx.QueryRow(ctx, `SELECT a.id, q.points::float8, a.submitted_at FROM quiz_answers x
		JOIN quiz_attempts a ON a.id=x.attempt_id JOIN quiz_questions q ON q.id=x.question_id WHERE x.id=$1 FOR UPDATE OF a`, answerID).Scan(&attemptID, &points, &submitted)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAttemptNotFound
	}
	if err != nil {
		return err
	}
	if submitted == nil {
		return fmt.Errorf("%w: attempt is not submitted", ErrInvalidGrade)
	}
	if score != nil && (*score < 0 || *score > points) {
		return fmt.Errorf("%w: score must be between 0 and %g", ErrInvalidGrade, points)
	}
	var grader any
	if graderID != "" {
		grader = graderID
	}
	if _, err := tx.Exec(ctx, `UPDATE quiz_answers SET manual_score=$2, grader_note=$3, graded_by=$4,
		graded_at = CASE WHEN $2::numeric IS NULL THEN NULL ELSE now() END WHERE id=$1`, answerID, score, strings.TrimSpace(note), grader); err != nil {
		return err
	}
	if err := recomputeTotals(ctx, tx, attemptID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type ExportRow struct {
	AttemptID        string
	UserID           string
	Cohort           string
	AttemptNumber    int
	QuizVersion      int
	StartedAt        time.Time
	SubmittedAt      *time.Time
	SubmitReason     *string
	QuestionIndex    int
	QuestionID       string
	QuestionType     string
	Prompt           string
	Selected         []string
	SelectedText     []string
	AnswerText       string
	Points           float64
	AutoScore        *float64
	ManualScore      *float64
	FinalScore       *float64
	IsCorrect        *bool
	NeedsReview      bool
	GraderNote       string
	TimeOnQuestionMs int64
	FirstSeenAt      time.Time
	AnsweredAt       *time.Time
	Revisions        int
}

// QuizExportRows returns one row per attempt × question for submitted attempts.
// Rows carry the pseudonymous user id and cohort, not the email address.
func (s *Store) QuizExportRows(ctx context.Context, quizID string) ([]ExportRow, error) {
	if _, err := uuid.Parse(quizID); err != nil {
		return nil, ErrQuizNotFound
	}
	if err := s.expireQuizAttempts(ctx, quizID, time.Now()); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id, a.user_id, u.cohort, a.attempt_number, a.quiz_version, a.started_at, a.submitted_at,
		a.submit_reason, array_position(a.question_order, q.id)-1, q.id, q.type, q.prompt, q.options, x.selected, x.answer_text,
		q.points::float8, x.auto_score::float8, x.manual_score::float8, COALESCE(x.manual_score, x.auto_score)::float8, x.is_correct,
		x.needs_review AND x.manual_score IS NULL, x.grader_note, x.time_on_question_ms, x.first_seen_at, x.answered_at, x.revisions
		FROM quiz_answers x JOIN quiz_attempts a ON a.id=x.attempt_id JOIN quiz_questions q ON q.id=x.question_id
		JOIN users u ON u.id=a.user_id WHERE a.quiz_id=$1 AND a.submitted_at IS NOT NULL
		ORDER BY a.started_at, a.id, array_position(a.question_order, q.id)`, quizID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportRow{}
	for rows.Next() {
		var r ExportRow
		var opts []byte
		if err := rows.Scan(&r.AttemptID, &r.UserID, &r.Cohort, &r.AttemptNumber, &r.QuizVersion, &r.StartedAt, &r.SubmittedAt,
			&r.SubmitReason, &r.QuestionIndex, &r.QuestionID, &r.QuestionType, &r.Prompt, &opts, &r.Selected, &r.AnswerText,
			&r.Points, &r.AutoScore, &r.ManualScore, &r.FinalScore, &r.IsCorrect, &r.NeedsReview, &r.GraderNote,
			&r.TimeOnQuestionMs, &r.FirstSeenAt, &r.AnsweredAt, &r.Revisions); err != nil {
			return nil, err
		}
		var options []quiz.Option
		_ = json.Unmarshal(opts, &options)
		text := map[string]string{}
		for _, o := range options {
			text[o.ID] = o.Text
		}
		for _, id := range r.Selected {
			r.SelectedText = append(r.SelectedText, text[id])
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type QuestionStat struct {
	QuestionID   string            `json:"question_id"`
	Position     int               `json:"position"`
	Type         quiz.QuestionType `json:"type"`
	Prompt       string            `json:"prompt"`
	Points       float64           `json:"points"`
	Answered     int               `json:"answered"`
	Blank        int               `json:"blank"`
	Pending      int               `json:"pending_review"`
	MeanScore    *float64          `json:"mean_score,omitempty"`
	PercentRight *float64          `json:"percent_correct,omitempty"`
	MedianTimeMs *float64          `json:"median_time_ms,omitempty"`
}

func (s *Store) QuizQuestionStats(ctx context.Context, quizID string) ([]QuestionStat, error) {
	if _, err := uuid.Parse(quizID); err != nil {
		return nil, ErrQuizNotFound
	}
	if err := s.expireQuizAttempts(ctx, quizID, time.Now()); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT q.id, q.position, q.type, q.prompt, q.points::float8,
		count(x.id) FILTER (WHERE x.answer_text <> '' OR cardinality(x.selected) > 0),
		count(x.id) FILTER (WHERE x.answer_text = '' AND cardinality(x.selected) = 0),
		count(x.id) FILTER (WHERE x.needs_review AND x.manual_score IS NULL),
		avg(COALESCE(x.manual_score, x.auto_score))::float8,
		(avg(CASE WHEN x.is_correct THEN 1.0 WHEN x.is_correct IS NOT NULL THEN 0.0 END)*100)::float8,
		percentile_cont(0.5) WITHIN GROUP (ORDER BY x.time_on_question_ms)::float8
		FROM quiz_questions q
		LEFT JOIN quiz_answers x ON x.question_id=q.id AND EXISTS (SELECT 1 FROM quiz_attempts a WHERE a.id=x.attempt_id AND a.submitted_at IS NOT NULL)
		WHERE q.quiz_id=$1 GROUP BY q.id ORDER BY q.position`, quizID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QuestionStat{}
	for rows.Next() {
		var st QuestionStat
		if err := rows.Scan(&st.QuestionID, &st.Position, &st.Type, &st.Prompt, &st.Points, &st.Answered, &st.Blank, &st.Pending,
			&st.MeanScore, &st.PercentRight, &st.MedianTimeMs); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
