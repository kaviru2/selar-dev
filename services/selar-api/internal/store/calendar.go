package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/calendar"
	"github.com/selar-dev/selar-api/internal/quiz"
)

// CalendarFeedState is a learner's calendar-feed opt-in.
type CalendarFeedState struct {
	Email   string
	Enabled bool
	Version int
}

// GetCalendarFeedState returns the learner's opt-in and feed version.
func (s *Store) GetCalendarFeedState(ctx context.Context, userID string) (CalendarFeedState, error) {
	var st CalendarFeedState
	err := s.pool.QueryRow(ctx, `SELECT email, calendar_feed_enabled, calendar_feed_version FROM users WHERE id = $1`, userID).
		Scan(&st.Email, &st.Enabled, &st.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, ErrUserNotFound
	}
	return st, err
}

// SetCalendarFeed turns the feed on or off. Turning it off, and resetting
// the link (rotate), increment the version so every earlier URL stops
// returning events.
func (s *Store) SetCalendarFeed(ctx context.Context, userID string, enabled, rotate bool) (CalendarFeedState, error) {
	var st CalendarFeedState
	err := s.pool.QueryRow(ctx, `UPDATE users SET calendar_feed_enabled = $2,
		calendar_feed_version = calendar_feed_version + CASE WHEN $3 OR (calendar_feed_enabled AND NOT $2) THEN 1 ELSE 0 END
		WHERE id = $1 RETURNING email, calendar_feed_enabled, calendar_feed_version`, userID, enabled, rotate).
		Scan(&st.Email, &st.Enabled, &st.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, ErrUserNotFound
	}
	return st, err
}

// CalendarWindows evaluates every published or closed quiz for the learner
// with the same rules the quiz API enforces (quiz.Evaluate) and returns the
// windows worth putting in a calendar. It only reads: open attempts are not
// expired here, which cannot change the result because Evaluate treats a
// past close time as closed regardless of attempt state.
func (s *Store) CalendarWindows(ctx context.Context, userID string, now time.Time) ([]calendar.Window, error) {
	learner, err := s.learnerContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	stats, err := s.attemptStatsByQuiz(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, kind, status, settings, updated_at FROM quizzes WHERE status <> 'draft'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []calendar.Window{}
	for rows.Next() {
		var id string
		var kind quiz.Kind
		var status quiz.Status
		var raw []byte
		var updated time.Time
		if err := rows.Scan(&id, &kind, &status, &raw, &updated); err != nil {
			return nil, err
		}
		var settings quiz.Settings
		if json.Unmarshal(raw, &settings) != nil {
			continue
		}
		st := stats[id]
		if st == nil {
			st = &attemptStats{}
		}
		a := quiz.Evaluate(quiz.Quiz{ID: id, Status: status, Kind: kind, Settings: settings}, learner,
			quiz.Attempts{Used: st.used, InProgress: st.openID != ""}, now)
		if w, ok := calendar.FromAvailability(id, kind, a, updated); ok {
			out = append(out, w)
		}
	}
	return out, rows.Err()
}
