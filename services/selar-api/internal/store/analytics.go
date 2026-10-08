package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/analytics"
)

// ============================================================
// Roles, consent and group labels
// ============================================================

// ErrInvalidGroupLabel is returned for over-long or control-character labels.
var ErrInvalidGroupLabel = errors.New("group label must be at most 64 printable characters")

// GetUserRole returns "user" or "admin".
func (s *Store) GetUserRole(ctx context.Context, userID string) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&role)
	return role, err
}

func validRole(role string) bool { return role == "user" || role == "admin" }

// SetUserRoleByEmail changes a user's role. found is false when no account
// has that email (matched case-insensitively).
func (s *Store) SetUserRoleByEmail(ctx context.Context, email, role string) (found bool, err error) {
	if !validRole(role) {
		return false, fmt.Errorf("unknown role %q", role)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET role = $2 WHERE lower(email) = lower($1)`, strings.TrimSpace(email), role)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListAdminEmails lists the emails of all admins (for the admin CLI).
func (s *Store) ListAdminEmails(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT email FROM users WHERE role = 'admin' ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		out = append(out, email)
	}
	return out, rows.Err()
}

// SetUserRole changes a user's role by id.
func (s *Store) SetUserRole(ctx context.Context, userID, role string) error {
	if !validRole(role) {
		return fmt.Errorf("unknown role %q", role)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, userID, role)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return err
}

// SetResearchConsent records the user's optional analytics opt-in (granted)
// or opt-out/withdrawal. Either way the decision time is stored so the prompt
// is not shown again. Withdrawal does not delete earlier events; that is the
// separate DeleteUserAnalytics action.
func (s *Store) SetResearchConsent(ctx context.Context, userID string, granted bool, version string) error {
	var tag interface{ RowsAffected() int64 }
	var err error
	if granted {
		if version == "" {
			return errors.New("consent version required")
		}
		tag, err = s.pool.Exec(ctx, `UPDATE users SET consented_at = now(), consent_version = $2, consent_decided_at = now() WHERE id = $1`, userID, version)
	} else {
		tag, err = s.pool.Exec(ctx, `UPDATE users SET consented_at = NULL, consent_version = NULL, consent_decided_at = now() WHERE id = $1`, userID)
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return err
}

// NormalizeGroupLabel trims and validates an admin-entered group label.
func NormalizeGroupLabel(label string) (string, error) {
	label = strings.TrimSpace(label)
	if len([]rune(label)) > 64 {
		return "", ErrInvalidGroupLabel
	}
	for _, r := range label {
		if unicode.IsControl(r) || r == 0 {
			return "", ErrInvalidGroupLabel
		}
	}
	return label, nil
}

// SetUserGroupLabel sets the neutral, admin-only group label ("" clears it).
func (s *Store) SetUserGroupLabel(ctx context.Context, userID, label string) error {
	label, err := NormalizeGroupLabel(label)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET group_label = $2 WHERE id = $1`, userID, label)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return err
}

// ============================================================
// Event recording
// ============================================================

// EventInput is one event to record. Props are sanitised again here.
type EventInput struct {
	Event      analytics.Event
	Props      analytics.Props
	OccurredAt time.Time
}

// RecordEvents stores a batch of events for userID in one statement:
//   - every valid event increments the anonymous per-day counter (UTC day,
//     no user id), and
//   - a per-user row is written only if the user has research consent at the
//     moment of the write (checked inside the same statement, so a withdrawal
//     that commits first wins).
//
// Unknown events and server-only events from a client source are skipped.
// It returns the number of per-user rows written.
func (s *Store) RecordEvents(ctx context.Context, userID string, source analytics.Source, events []EventInput) (int64, error) {
	names := make([]string, 0, len(events))
	props := make([]string, 0, len(events))
	times := make([]time.Time, 0, len(events))
	var countOnly []string
	for _, e := range events {
		clean, err := analytics.Sanitize(e.Event, e.Props, source)
		if err != nil {
			continue
		}
		raw, err := json.Marshal(clean)
		if err != nil {
			continue
		}
		at := e.OccurredAt
		if at.IsZero() {
			at = time.Now()
		}
		names = append(names, string(e.Event))
		props = append(props, string(raw))
		times = append(times, at.UTC())
		if spec, _ := analytics.Lookup(e.Event); spec.CountOnly {
			countOnly = append(countOnly, string(e.Event))
		}
	}
	if len(names) == 0 {
		return 0, nil
	}
	if countOnly == nil {
		countOnly = []string{}
	}
	tag, err := s.pool.Exec(ctx, `
		WITH input AS (
		  SELECT t.event, t.props::jsonb AS props, t.occurred_at
		  FROM unnest($2::text[], $3::text[], $4::timestamptz[]) AS t(event, props, occurred_at)
		), counted AS (
		  INSERT INTO analytics_daily_counts (day, event, count)
		  SELECT (occurred_at AT TIME ZONE 'UTC')::date, event, count(*) FROM input GROUP BY 1, 2
		  ON CONFLICT (day, event) DO UPDATE SET count = analytics_daily_counts.count + EXCLUDED.count
		  RETURNING 1
		)
		INSERT INTO analytics_events (user_id, event, props, source, occurred_at)
		SELECT u.id, i.event, i.props, $5, i.occurred_at
		FROM input i JOIN users u ON u.id = $1 AND u.consented_at IS NOT NULL
		WHERE NOT (i.event = ANY($6::text[]))`,
		userID, names, props, times, string(source), countOnly)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DeleteUserAnalytics removes every per-user analytics event of userID and
// returns how many were deleted. Anonymous daily counts are kept (they hold
// no user id).
func (s *Store) DeleteUserAnalytics(ctx context.Context, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM analytics_events WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	_, _ = s.RecordEvents(ctx, userID, analytics.SourceServer, []EventInput{{Event: analytics.AnalyticsDataDeleted}})
	return tag.RowsAffected(), nil
}

// ============================================================
// Admin aggregates (server-side, index-backed)
// ============================================================

// GroupNone filters users without a group label.
const GroupNone = "__none__"

// AdminFilter restricts admin queries to a time window and optional group.
type AdminFilter struct {
	From  time.Time
	To    time.Time
	Group string // "" = all users, GroupNone = unlabelled users
}

// groupClause matches users u against the filter's group ($n placeholder).
func groupClause(alias string, placeholder int) string {
	return fmt.Sprintf(`($%d = '' OR (%s.group_label = $%d) OR ($%d = '%s' AND %s.group_label = ''))`,
		placeholder, alias, placeholder, placeholder, GroupNone, alias)
}

// DecisionCounts splits suggestion decisions by action.
type DecisionCounts struct {
	Total   int64 `json:"total"`
	Keep    int64 `json:"keep"`
	Change  int64 `json:"change"`
	Reject  int64 `json:"reject"`
	Retract int64 `json:"retract"`
	Undo    int64 `json:"undo"`
}

// DailyPoint is one day of the overview time series (UTC days).
type DailyPoint struct {
	Day           string `json:"day"`
	ActiveUsers   int64  `json:"active_users"`
	Events        int64  `json:"events"`
	Decisions     int64  `json:"decisions"`
	ReadingMs     int64  `json:"reading_ms"`
	ChatQuestions int64  `json:"chat_questions"`
}

// AdminOverview holds the dashboard KPIs for a filter. Per-user figures come
// only from consenting users; AnonymousCounts covers everyone without ids.
type AdminOverview struct {
	From              time.Time        `json:"from"`
	To                time.Time        `json:"to"`
	Group             string           `json:"group"`
	TotalUsers        int64            `json:"total_users"`
	ConsentedUsers    int64            `json:"consented_users"`
	AdminUsers        int64            `json:"admin_users"`
	ActiveUsers       int64            `json:"active_users"`
	ActiveUsersLast1d int64            `json:"active_users_last_1d"`
	ActiveUsersLast7d int64            `json:"active_users_last_7d"`
	Documents         int64            `json:"documents"`
	DocumentsAdded    int64            `json:"documents_added"`
	SuggestionsShown  int64            `json:"suggestions_shown"`
	Decisions         DecisionCounts   `json:"decisions"`
	ReadingMs         int64            `json:"reading_ms"`
	ReadingSessions   int64            `json:"reading_sessions"`
	ChatQuestions     int64            `json:"chat_questions"`
	ChatUsers         int64            `json:"chat_users"`
	QuizSubmissions   int64            `json:"quiz_submissions"`
	Daily             []DailyPoint     `json:"daily"`
	Groups            []GroupCount     `json:"groups"`
	AnonymousCounts   map[string]int64 `json:"anonymous_counts"`
}

// GroupCount lists the known group labels with user counts.
type GroupCount struct {
	Label string `json:"label"`
	Users int64  `json:"users"`
}

// AdminOverview computes KPIs with a handful of aggregate queries.
func (s *Store) AdminOverview(ctx context.Context, f AdminFilter) (*AdminOverview, error) {
	o := &AdminOverview{From: f.From, To: f.To, Group: f.Group, AnonymousCounts: map[string]int64{}}
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE u.consented_at IS NOT NULL), count(*) FILTER (WHERE u.role = 'admin'),
		       (SELECT count(*) FROM documents d JOIN users du ON du.id = d.user_id WHERE `+groupClause("du", 1)+`),
		       (SELECT count(*) FROM documents d JOIN users du ON du.id = d.user_id
		         WHERE `+groupClause("du", 1)+` AND d.added_at >= $2 AND d.added_at < $3)
		FROM users u WHERE `+groupClause("u", 1), f.Group, f.From, f.To).
		Scan(&o.TotalUsers, &o.ConsentedUsers, &o.AdminUsers, &o.Documents, &o.DocumentsAdded); err != nil {
		return nil, err
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT e.user_id),
		       count(DISTINCT e.user_id) FILTER (WHERE e.occurred_at >= $3::timestamptz - interval '1 day'),
		       count(DISTINCT e.user_id) FILTER (WHERE e.occurred_at >= $3::timestamptz - interval '7 days'),
		       count(*) FILTER (WHERE e.event = 'suggestion_shown'),
		       count(*) FILTER (WHERE e.event = 'decision_made'),
		       count(*) FILTER (WHERE e.event = 'decision_made' AND e.props->>'action' = 'keep'),
		       count(*) FILTER (WHERE e.event = 'decision_made' AND e.props->>'action' = 'change'),
		       count(*) FILTER (WHERE e.event = 'decision_made' AND e.props->>'action' = 'reject'),
		       count(*) FILTER (WHERE e.event = 'decision_made' AND e.props->>'action' = 'retract'),
		       count(*) FILTER (WHERE e.event = 'decision_made' AND e.props->>'action' = 'undo'),
		       coalesce(sum((e.props->>'dwell_ms')::bigint) FILTER (WHERE e.event = 'reader_page_viewed'), 0),
		       count(*) FILTER (WHERE e.event = 'reading_session_started'),
		       count(*) FILTER (WHERE e.event = 'chat_question_asked'),
		       count(DISTINCT e.user_id) FILTER (WHERE e.event = 'chat_question_asked'),
		       count(*) FILTER (WHERE e.event = 'quiz_submitted')
		FROM analytics_events e JOIN users u ON u.id = e.user_id
		WHERE e.occurred_at >= $2 AND e.occurred_at < $3 AND `+groupClause("u", 1), f.Group, f.From, f.To).
		Scan(&o.ActiveUsers, &o.ActiveUsersLast1d, &o.ActiveUsersLast7d, &o.SuggestionsShown,
			&o.Decisions.Total, &o.Decisions.Keep, &o.Decisions.Change, &o.Decisions.Reject, &o.Decisions.Retract, &o.Decisions.Undo,
			&o.ReadingMs, &o.ReadingSessions, &o.ChatQuestions, &o.ChatUsers, &o.QuizSubmissions); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		WITH days AS (
		  SELECT generate_series(($2::timestamptz AT TIME ZONE 'UTC')::date,
		                         (($3::timestamptz - interval '1 microsecond') AT TIME ZONE 'UTC')::date, interval '1 day')::date AS day
		), agg AS (
		  SELECT (e.occurred_at AT TIME ZONE 'UTC')::date AS day,
		         count(DISTINCT e.user_id) AS active, count(*) AS events,
		         count(*) FILTER (WHERE e.event = 'decision_made') AS decisions,
		         coalesce(sum((e.props->>'dwell_ms')::bigint) FILTER (WHERE e.event = 'reader_page_viewed'), 0) AS reading,
		         count(*) FILTER (WHERE e.event = 'chat_question_asked') AS chat
		  FROM analytics_events e JOIN users u ON u.id = e.user_id
		  WHERE e.occurred_at >= $2 AND e.occurred_at < $3 AND `+groupClause("u", 1)+`
		  GROUP BY 1
		)
		SELECT to_char(d.day, 'YYYY-MM-DD'), coalesce(a.active, 0), coalesce(a.events, 0), coalesce(a.decisions, 0),
		       coalesce(a.reading, 0), coalesce(a.chat, 0)
		FROM days d LEFT JOIN agg a ON a.day = d.day ORDER BY d.day
		LIMIT 400`, f.Group, f.From, f.To)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p DailyPoint
		if err := rows.Scan(&p.Day, &p.ActiveUsers, &p.Events, &p.Decisions, &p.ReadingMs, &p.ChatQuestions); err != nil {
			rows.Close()
			return nil, err
		}
		o.Daily = append(o.Daily, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	groups, err := s.pool.Query(ctx, `SELECT group_label, count(*) FROM users GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	for groups.Next() {
		var g GroupCount
		if err := groups.Scan(&g.Label, &g.Users); err != nil {
			groups.Close()
			return nil, err
		}
		o.Groups = append(o.Groups, g)
	}
	groups.Close()

	anon, err := s.pool.Query(ctx, `SELECT event, sum(count) FROM analytics_daily_counts
		WHERE day >= ($1::timestamptz AT TIME ZONE 'UTC')::date AND day <= (($2::timestamptz - interval '1 microsecond') AT TIME ZONE 'UTC')::date
		GROUP BY event`, f.From, f.To)
	if err != nil {
		return nil, err
	}
	defer anon.Close()
	for anon.Next() {
		var event string
		var n int64
		if err := anon.Scan(&event, &n); err != nil {
			return nil, err
		}
		o.AnonymousCounts[event] = n
	}
	return o, anon.Err()
}

// AdminUserRow is one row of the admin users table. Activity figures come
// from consented analytics events only.
type AdminUserRow struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Role            string     `json:"role"`
	GroupLabel      string     `json:"group_label"`
	CreatedAt       time.Time  `json:"created_at"`
	ConsentedAt     *time.Time `json:"consented_at"`
	ConsentVersion  *string    `json:"consent_version"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	ActiveDays      int64      `json:"active_days"`
	Events          int64      `json:"events"`
	Documents       int64      `json:"documents"`
	ReadingMs       int64      `json:"reading_ms"`
	SuggestionsSeen int64      `json:"suggestions_shown"`
	Decisions       int64      `json:"decisions"`
	Keep            int64      `json:"keep"`
	Change          int64      `json:"change"`
	Reject          int64      `json:"reject"`
	ChatQuestions   int64      `json:"chat_questions"`
	QuizAttempts    int64      `json:"quiz_attempts"`
	QuizAvgPct      *float64   `json:"quiz_avg_pct"`
}

const adminUserSelect = `
	SELECT u.id, u.email, u.role, u.group_label, u.created_at, u.consented_at, u.consent_version,
	       a.last_seen, coalesce(a.active_days, 0), coalesce(a.events, 0),
	       (SELECT count(*) FROM documents d WHERE d.user_id = u.id),
	       coalesce(a.reading, 0), coalesce(a.shown, 0), coalesce(a.decisions, 0),
	       coalesce(a.keep, 0), coalesce(a.change, 0), coalesce(a.reject, 0), coalesce(a.chat, 0),
	       coalesce(a.quizzes, 0), a.quiz_avg
	FROM users u
	LEFT JOIN LATERAL (
	  SELECT max(e.occurred_at) AS last_seen,
	         count(DISTINCT (e.occurred_at AT TIME ZONE 'UTC')::date) AS active_days,
	         count(*) AS events,
	         sum((e.props->>'dwell_ms')::bigint) FILTER (WHERE e.event = 'reader_page_viewed') AS reading,
	         count(*) FILTER (WHERE e.event = 'suggestion_shown') AS shown,
	         count(*) FILTER (WHERE e.event = 'decision_made') AS decisions,
	         count(*) FILTER (WHERE e.event = 'decision_made' AND e.props->>'action' = 'keep') AS keep,
	         count(*) FILTER (WHERE e.event = 'decision_made' AND e.props->>'action' = 'change') AS change,
	         count(*) FILTER (WHERE e.event = 'decision_made' AND e.props->>'action' = 'reject') AS reject,
	         count(*) FILTER (WHERE e.event = 'chat_question_asked') AS chat,
	         count(*) FILTER (WHERE e.event = 'quiz_submitted') AS quizzes,
	         avg((e.props->>'score_pct')::numeric) FILTER (WHERE e.event = 'quiz_submitted' AND e.props ? 'score_pct')::float8 AS quiz_avg
	  FROM analytics_events e
	  WHERE e.user_id = u.id AND e.occurred_at >= $2 AND e.occurred_at < $3
	) a ON true`

func scanAdminUser(row interface{ Scan(...any) error }) (AdminUserRow, error) {
	var r AdminUserRow
	err := row.Scan(&r.ID, &r.Email, &r.Role, &r.GroupLabel, &r.CreatedAt, &r.ConsentedAt, &r.ConsentVersion,
		&r.LastSeenAt, &r.ActiveDays, &r.Events, &r.Documents, &r.ReadingMs, &r.SuggestionsSeen, &r.Decisions,
		&r.Keep, &r.Change, &r.Reject, &r.ChatQuestions, &r.QuizAttempts, &r.QuizAvgPct)
	return r, err
}

// AdminListUsers lists users matching the group filter with activity in the
// date window, most recently active first.
func (s *Store) AdminListUsers(ctx context.Context, f AdminFilter) ([]AdminUserRow, error) {
	rows, err := s.pool.Query(ctx, adminUserSelect+` WHERE `+groupClause("u", 1)+`
		ORDER BY a.last_seen DESC NULLS LAST, u.created_at DESC LIMIT 1000`, f.Group, f.From, f.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUserRow{}
	for rows.Next() {
		r, err := scanAdminUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AdminGetUser returns one user's admin row.
func (s *Store) AdminGetUser(ctx context.Context, userID string, f AdminFilter) (*AdminUserRow, error) {
	r, err := scanAdminUser(s.pool.QueryRow(ctx, adminUserSelect+` WHERE u.id = $4 AND $1::text = $1::text`, "", f.From, f.To, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// TimelineEvent is one analytics event in a user's drill-down.
type TimelineEvent struct {
	ID         int64           `json:"id"`
	Event      string          `json:"event"`
	Source     string          `json:"source"`
	Props      json.RawMessage `json:"props"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// AdminUserTimeline returns a user's events in the window, newest first.
func (s *Store) AdminUserTimeline(ctx context.Context, userID string, f AdminFilter, limit int) ([]TimelineEvent, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `SELECT id, event, source, props, occurred_at FROM analytics_events
		WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at < $3
		ORDER BY occurred_at DESC, id DESC LIMIT $4`, userID, f.From, f.To, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TimelineEvent{}
	for rows.Next() {
		var e TimelineEvent
		if err := rows.Scan(&e.ID, &e.Event, &e.Source, &e.Props, &e.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ExportEventRow is one CSV row of the event export.
type ExportEventRow struct {
	OccurredAt time.Time
	UserID     string
	Email      string // only filled when the admin explicitly asked for emails
	GroupLabel string
	Event      string
	Source     string
	Props      string
}

// AdminExportEvents streams events in the window (oldest first) to fn.
// Emails are selected only when includeEmail is true; passwords never are.
func (s *Store) AdminExportEvents(ctx context.Context, f AdminFilter, includeEmail bool, fn func(ExportEventRow) error) error {
	rows, err := s.pool.Query(ctx, `SELECT e.occurred_at, e.user_id::text, CASE WHEN $4 THEN u.email ELSE '' END,
		u.group_label, e.event, e.source, e.props::text
		FROM analytics_events e JOIN users u ON u.id = e.user_id
		WHERE e.occurred_at >= $2 AND e.occurred_at < $3 AND `+groupClause("u", 1)+`
		ORDER BY e.occurred_at, e.id LIMIT 500000`, f.Group, f.From, f.To, includeEmail)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r ExportEventRow
		if err := rows.Scan(&r.OccurredAt, &r.UserID, &r.Email, &r.GroupLabel, &r.Event, &r.Source, &r.Props); err != nil {
			return err
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return rows.Err()
}

// QuizAttemptFacts is what the analytics layer records about a quiz attempt.
type QuizAttemptFacts struct {
	QuizID        string
	Kind          string
	QuestionCount int
	Submitted     bool
	ScorePct      *int
	DurationMs    int64
}

// QuizAttemptFactsFor returns analytics facts for an attempt owned by userID.
func (s *Store) QuizAttemptFactsFor(ctx context.Context, userID, attemptID string) (*QuizAttemptFacts, error) {
	var f QuizAttemptFacts
	var score *float64
	err := s.pool.QueryRow(ctx, `
		SELECT a.quiz_id::text, q.kind, cardinality(a.question_order), a.submitted_at IS NOT NULL,
		       CASE WHEN a.score IS NOT NULL AND a.max_points > 0 THEN (a.score / a.max_points * 100)::float8 END,
		       (EXTRACT(EPOCH FROM (COALESCE(a.submitted_at, now()) - a.started_at)) * 1000)::bigint
		  FROM quiz_attempts a JOIN quizzes q ON q.id = a.quiz_id
		 WHERE a.id = $1 AND a.user_id = $2`, attemptID, userID).
		Scan(&f.QuizID, &f.Kind, &f.QuestionCount, &f.Submitted, &score, &f.DurationMs)
	if err != nil {
		return nil, err
	}
	if score != nil {
		pct := int(*score + 0.5)
		if pct > 100 {
			pct = 100
		}
		f.ScorePct = &pct
	}
	return &f, nil
}
