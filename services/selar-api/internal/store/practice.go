package store

// Read-only practice queries served directly from the API's pooled
// connection. They mirror selar-worker/practice_service.py exactly (same
// "live source" filter, same UTC streak rule) so the daily review, progress
// and reader warm-up no longer pay a Modal round trip plus a fresh database
// connection on every page view. Anything that calls the AI provider
// (generating new items, grading attempts) stays in the worker.

import (
	"context"
	"encoding/json"
	"time"
)

// practiceLive keeps only items whose source snapshot is unchanged: the
// document is ready with the same content hash, and the chunk text, locator
// and quoted witness still match. Must stay identical to LIVE in
// selar-worker/practice_service.py.
const practiceLive = `FROM practice_items p JOIN documents d ON d.id=p.document_id AND d.user_id=p.user_id
 JOIN chunks c ON c.id=p.chunk_id AND c.user_id=p.user_id AND c.document_id=d.id
 WHERE p.user_id=$1 AND d.status='ready' AND d.content_hash=p.document_hash
 AND encode(sha256(convert_to(c.content,'UTF8')),'hex')=p.chunk_hash
 AND c.locator=p.payload->'locator' AND c.locator<>'{}'::jsonb
 AND position(p.payload->>'quote' in c.content)>0`

const practiceDue = ` AND EXISTS(SELECT 1 FROM practice_schedule s WHERE s.item_id=p.id AND s.user_id=$1 AND s.due_at<=now())`

// PracticeItem is the learner-visible part of a practice item (never the
// reference answer or quote).
type PracticeItem struct {
	ID         string          `json:"id"`
	DocumentID string          `json:"document_id"`
	Question   string          `json:"question"`
	Label      string          `json:"label"`
	Version    json.RawMessage `json:"version,omitempty"`
}

// PracticeDaily is the daily review queue.
type PracticeDaily struct {
	Status   string         `json:"status"`
	Items    []PracticeItem `json:"items"`
	Streak   int            `json:"streak"`
	Timezone string         `json:"timezone"`
	Policy   string         `json:"policy"`
}

// PracticeProgress is the learner's own practice report.
type PracticeProgress struct {
	Status            string   `json:"status"`
	Warmup            int      `json:"warmup"`
	ReadingCheck      int      `json:"reading_check"`
	Review            int      `json:"review"`
	Exposed           int      `json:"exposed"`
	DelayedUnassisted int      `json:"delayed_unassisted"`
	DelayedScored     int      `json:"delayed_scored"`
	Unscored          int      `json:"unscored"`
	DelayedMeanScore  *float64 `json:"delayed_mean_score"`
	EstimatedRecall   *float64 `json:"estimated_recall"`
	EstimateNote      string   `json:"estimate_note"`
	Streak            int      `json:"streak"`
	Due               int      `json:"due"`
	Timezone          string   `json:"timezone"`
}

const (
	practicePolicy       = "doubling-interval-v1 (heuristic, not a recall estimate)"
	practiceEstimateNote = "No calibrated recall estimate is available. Scheduling intervals are a heuristic, not efficacy evidence."
)

// PracticeStreak counts consecutive UTC calendar days with a review attempt,
// ending today (or yesterday when today has none yet). Mirrors
// practice_schedule.streak in the worker.
func PracticeStreak(days []time.Time, today time.Time) int {
	set := make(map[string]bool, len(days))
	for _, d := range days {
		set[d.UTC().Format("2006-01-02")] = true
	}
	day := today.UTC()
	if !set[day.Format("2006-01-02")] {
		day = day.AddDate(0, 0, -1)
	}
	count := 0
	for set[day.Format("2006-01-02")] {
		count++
		day = day.AddDate(0, 0, -1)
	}
	return count
}

// practiceStreakSQL returns today's UTC date and the distinct UTC days with a
// review attempt in one round trip (aggregates always yield one row).
const practiceStreakSQL = `SELECT (now() AT TIME ZONE 'UTC')::date,
	COALESCE(array_agg(DISTINCT (created_at AT TIME ZONE 'UTC')::date), '{}')
	FROM practice_attempts WHERE user_id=$1 AND phase='review' AND created_at<=now()`

func (s *Store) practiceStreak(ctx context.Context, owner string) (int, error) {
	var today time.Time
	var days []time.Time
	if err := s.pool.QueryRow(ctx, practiceStreakSQL, owner).Scan(&today, &days); err != nil {
		return 0, err
	}
	return PracticeStreak(days, today), nil
}

type streakResult struct {
	n   int
	err error
}

// goStreak computes the streak on its own pooled connection.
func (s *Store) goStreak(ctx context.Context, owner string) <-chan streakResult {
	out := make(chan streakResult, 1)
	go func() {
		n, err := s.practiceStreak(ctx, owner)
		out <- streakResult{n, err}
	}()
	return out
}

func (s *Store) practiceItems(ctx context.Context, query string, withVersion bool, args ...any) ([]PracticeItem, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PracticeItem{}
	for rows.Next() {
		var it PracticeItem
		var version []byte
		if err := rows.Scan(&it.ID, &it.DocumentID, &it.Question, &it.Label, &version); err != nil {
			return nil, err
		}
		if withVersion && len(version) > 0 {
			it.Version = json.RawMessage(version)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

const practiceItemColumns = `SELECT p.id::text, p.document_id::text, COALESCE(p.payload->>'question',''), COALESCE(p.payload->>'label',''), p.payload->'version' `

// PracticeDailyQueue returns up to 20 due, still-live items and the streak.
func (s *Store) PracticeDailyQueue(ctx context.Context, owner string) (*PracticeDaily, error) {
	// The queue and the streak are independent: run them concurrently on two
	// pooled connections so the page waits for one round trip, not two.
	streak := s.goStreak(ctx, owner)
	items, err := s.practiceItems(ctx, practiceItemColumns+practiceLive+practiceDue+` ORDER BY p.created_at,p.id LIMIT 20`, false, owner)
	st := <-streak
	if err != nil {
		return nil, err
	}
	if st.err != nil {
		return nil, st.err
	}
	return &PracticeDaily{Status: "ready", Items: items, Streak: st.n, Timezone: "UTC", Policy: practicePolicy}, nil
}

// PracticeDocumentItems returns the owner's live items for one document.
func (s *Store) PracticeDocumentItems(ctx context.Context, owner, documentID string) ([]PracticeItem, error) {
	return s.practiceItems(ctx, practiceItemColumns+practiceLive+` AND p.document_id=$2 ORDER BY p.created_at,p.id`, true, owner, documentID)
}

// PracticeProgressReport aggregates the owner's attempts on live items.
func (s *Store) PracticeProgressReport(ctx context.Context, owner string) (*PracticeProgress, error) {
	r := &PracticeProgress{Status: "ready", EstimateNote: practiceEstimateNote, Timezone: "UTC"}
	streak := s.goStreak(ctx, owner)
	const scored = `jsonb_typeof(a.feedback->'feedback'->'score')='number'`
	err := s.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE a.phase='warmup'),
		count(*) FILTER (WHERE a.phase='reading_check'),
		count(*) FILTER (WHERE a.phase='review'),
		count(*) FILTER (WHERE a.exposed),
		count(*) FILTER (WHERE a.delayed_unassisted),
		count(*) FILTER (WHERE NOT `+scored+`),
		count(*) FILTER (WHERE a.delayed_unassisted AND `+scored+`),
		avg((a.feedback->'feedback'->>'score')::float8) FILTER (WHERE a.delayed_unassisted AND `+scored+`),
		(SELECT count(*) `+practiceLive+practiceDue+`)
		FROM practice_attempts a
		WHERE a.user_id=$1 AND a.created_at<=now() AND a.item_id IN (SELECT p.id `+practiceLive+`)`, owner).
		Scan(&r.Warmup, &r.ReadingCheck, &r.Review, &r.Exposed, &r.DelayedUnassisted, &r.Unscored, &r.DelayedScored, &r.DelayedMeanScore, &r.Due)
	st := <-streak
	if err != nil {
		return nil, err
	}
	if st.err != nil {
		return nil, st.err
	}
	r.Streak = st.n
	return r, nil
}
