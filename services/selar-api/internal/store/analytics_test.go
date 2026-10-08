package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/analytics"
)

func analyticsTestStore(t *testing.T) (*Store, *pgxpool.Pool, func(prefix string) string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	var created []string
	t.Cleanup(func() {
		for _, id := range created {
			_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, id)
		}
		pool.Close()
	})
	newUser := func(prefix string) string {
		t.Helper()
		var id string
		email := prefix + "-" + uuid.NewString() + "@example.test"
		if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES ($1,'x') RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		created = append(created, id)
		return id
	}
	return New(pool), pool, newUser
}

func countEvents(t *testing.T, pool *pgxpool.Pool, userID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM analytics_events WHERE user_id=$1`, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAnalyticsRecordsPerUserEventsOnlyWithConsent(t *testing.T) {
	s, pool, newUser := analyticsTestStore(t)
	ctx := context.Background()
	optedOut := newUser("analytics-out")
	optedIn := newUser("analytics-in")
	if err := s.SetResearchConsent(ctx, optedIn, true, analytics.ConsentVersion); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour * 400) // isolated past day
	at := day.Add(3 * time.Hour)
	var before int64
	_ = pool.QueryRow(ctx, `SELECT coalesce(sum(count),0) FROM analytics_daily_counts WHERE day=$1 AND event='chat_question_asked'`, day).Scan(&before)

	for _, id := range []string{optedOut, optedIn} {
		if _, err := s.RecordEvents(ctx, id, analytics.SourceServer, []EventInput{{Event: analytics.ChatQuestionAsked, Props: analytics.Props{"length": int64(12)}, OccurredAt: at}}); err != nil {
			t.Fatal(err)
		}
	}
	if n := countEvents(t, pool, optedOut); n != 0 {
		t.Fatalf("a user who did not opt in must have no per-user events, got %d", n)
	}
	if n := countEvents(t, pool, optedIn); n != 1 {
		t.Fatalf("an opted-in user must have exactly one event, got %d", n)
	}
	var after int64
	_ = pool.QueryRow(ctx, `SELECT coalesce(sum(count),0) FROM analytics_daily_counts WHERE day=$1 AND event='chat_question_asked'`, day).Scan(&after)
	if after-before != 2 {
		t.Fatalf("both events must be counted anonymously, delta = %d", after-before)
	}
}

func TestAnalyticsWithdrawingConsentStopsRecordingAndDeleteRemovesEvents(t *testing.T) {
	s, pool, newUser := analyticsTestStore(t)
	ctx := context.Background()
	id := newUser("analytics-withdraw")
	_ = s.SetResearchConsent(ctx, id, true, analytics.ConsentVersion)
	_, _ = s.RecordEvents(ctx, id, analytics.SourceClient, []EventInput{{Event: analytics.PageViewed, Props: analytics.Props{"route": "/library"}, OccurredAt: time.Now()}})
	if err := s.SetResearchConsent(ctx, id, false, ""); err != nil {
		t.Fatal(err)
	}
	_, _ = s.RecordEvents(ctx, id, analytics.SourceClient, []EventInput{{Event: analytics.PageViewed, Props: analytics.Props{"route": "/reader"}, OccurredAt: time.Now()}})
	if n := countEvents(t, pool, id); n != 1 {
		t.Fatalf("no events may be recorded after withdrawal, got %d", n)
	}
	deleted, err := s.DeleteUserAnalytics(ctx, id)
	if err != nil || deleted != 1 || countEvents(t, pool, id) != 0 {
		t.Fatalf("delete-my-data must remove all events: deleted=%d err=%v", deleted, err)
	}
	user, err := s.GetUserByID(ctx, id)
	if err != nil || user.ConsentedAt != nil || user.ConsentDecidedAt == nil {
		t.Fatalf("withdrawal must clear consent but remember the decision: %+v %v", user, err)
	}
}

func TestAnalyticsDeletingAUserDeletesTheirEvents(t *testing.T) {
	s, pool, newUser := analyticsTestStore(t)
	ctx := context.Background()
	id := newUser("analytics-cascade")
	_ = s.SetResearchConsent(ctx, id, true, analytics.ConsentVersion)
	_, _ = s.RecordEvents(ctx, id, analytics.SourceServer, []EventInput{{Event: analytics.SignedIn, Props: analytics.Props{"method": "login"}, OccurredAt: time.Now()}})
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, pool, id); n != 0 {
		t.Fatalf("user deletion must cascade to analytics events, %d left", n)
	}
}

func TestAnalyticsPromoteByEmailAndRoleLookup(t *testing.T) {
	s, pool, newUser := analyticsTestStore(t)
	ctx := context.Background()
	id := newUser("analytics-admin")
	var email string
	_ = pool.QueryRow(ctx, `SELECT email FROM users WHERE id=$1`, id).Scan(&email)
	if role, err := s.GetUserRole(ctx, id); err != nil || role != "user" {
		t.Fatalf("new users must default to role user, got %q %v", role, err)
	}
	found, err := s.SetUserRoleByEmail(ctx, "  "+email+" ", "admin")
	if err != nil || !found {
		t.Fatalf("promote by email failed: %v %v", found, err)
	}
	if role, _ := s.GetUserRole(ctx, id); role != "admin" {
		t.Fatalf("role = %q, want admin", role)
	}
	if found, _ := s.SetUserRoleByEmail(ctx, "nobody-"+uuid.NewString()+"@example.test", "admin"); found {
		t.Fatal("promoting an unknown email must report not found")
	}
	if _, err := s.SetUserRoleByEmail(ctx, email, "superuser"); err == nil {
		t.Fatal("unknown roles must be rejected")
	}
}

func TestAnalyticsAdminAggregatesFilterByGroupAndDate(t *testing.T) {
	s, _, newUser := analyticsTestStore(t)
	ctx := context.Background()
	a := newUser("analytics-agg-a")
	b := newUser("analytics-agg-b")
	group := "g-" + uuid.NewString()[:8]
	for _, id := range []string{a, b} {
		_ = s.SetResearchConsent(ctx, id, true, analytics.ConsentVersion)
	}
	if err := s.SetUserGroupLabel(ctx, a, group); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Hour)
	_, _ = s.RecordEvents(ctx, a, analytics.SourceServer, []EventInput{
		{Event: analytics.DecisionMade, Props: analytics.Props{"kind": "mental_link", "action": "keep"}, OccurredAt: at},
		{Event: analytics.DecisionMade, Props: analytics.Props{"kind": "mental_link", "action": "reject"}, OccurredAt: at},
		{Event: analytics.QuizSubmitted, Props: analytics.Props{"score_pct": int64(80)}, OccurredAt: at},
	})
	_, _ = s.RecordEvents(ctx, a, analytics.SourceClient, []EventInput{
		{Event: analytics.SuggestionShown, Props: analytics.Props{"kind": "mental_link"}, OccurredAt: at},
		{Event: analytics.ReaderPageViewed, Props: analytics.Props{"page": int64(1), "dwell_ms": int64(60000)}, OccurredAt: at},
	})
	_, _ = s.RecordEvents(ctx, b, analytics.SourceServer, []EventInput{
		{Event: analytics.DecisionMade, Props: analytics.Props{"kind": "mental_link", "action": "change"}, OccurredAt: at},
	})

	filter := AdminFilter{From: at.Add(-time.Hour), To: at.Add(time.Hour), Group: group}
	overview, err := s.AdminOverview(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if overview.ActiveUsers != 1 || overview.Decisions.Total != 2 || overview.Decisions.Keep != 1 ||
		overview.Decisions.Reject != 1 || overview.Decisions.Change != 0 || overview.SuggestionsShown != 1 || overview.ReadingMs != 60000 {
		t.Fatalf("group filter not applied: %+v", overview)
	}
	users, err := s.AdminListUsers(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != a || users[0].Decisions != 2 || users[0].QuizAttempts != 1 ||
		users[0].QuizAvgPct == nil || *users[0].QuizAvgPct != 80 || users[0].ReadingMs != 60000 {
		t.Fatalf("unexpected user rows: %+v", users)
	}
	old := AdminFilter{From: at.Add(-72 * time.Hour), To: at.Add(-48 * time.Hour), Group: group}
	if overview, _ := s.AdminOverview(ctx, old); overview.Decisions.Total != 0 {
		t.Fatalf("date filter not applied: %+v", overview)
	}
	timeline, err := s.AdminUserTimeline(ctx, a, filter, 100)
	if err != nil || len(timeline) != 5 {
		t.Fatalf("timeline = %d events, err %v", len(timeline), err)
	}
}

func TestAnalyticsGroupLabelValidation(t *testing.T) {
	s, _, newUser := analyticsTestStore(t)
	id := newUser("analytics-label")
	if err := s.SetUserGroupLabel(context.Background(), id, string(make([]byte, 65))); err == nil {
		t.Fatal("over-long or control-character labels must be rejected")
	}
	if err := s.SetUserGroupLabel(context.Background(), uuid.NewString(), "A"); err == nil {
		t.Fatal("labelling an unknown user must fail")
	}
}

func TestAnalyticsQuizAttemptFacts(t *testing.T) {
	s, pool, newUser := analyticsTestStore(t)
	ctx := context.Background()
	u := newUser("analytics-quiz")
	var quizID, attemptID string
	if err := pool.QueryRow(ctx, `INSERT INTO quizzes (title, kind) VALUES ('DEMO quiz', 'follow_up') RETURNING id::text`).Scan(&quizID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM quizzes WHERE id = $1`, quizID) })
	if err := pool.QueryRow(ctx, `
		INSERT INTO quiz_attempts (quiz_id, user_id, attempt_number, quiz_version, started_at, submitted_at, submit_reason,
		                           question_order, max_points, score)
		VALUES ($1, $2, 1, 1, now() - interval '90 seconds', now(), 'learner',
		        ARRAY[gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), gen_random_uuid()], 4, 3)
		RETURNING id::text`, quizID, u).Scan(&attemptID); err != nil {
		t.Fatal(err)
	}
	f, err := s.QuizAttemptFactsFor(ctx, u, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if f.QuizID != quizID || f.Kind != "follow_up" || f.QuestionCount != 4 || !f.Submitted || f.ScorePct == nil || *f.ScorePct != 75 {
		t.Fatalf("unexpected facts %+v", f)
	}
	if f.DurationMs < 89_000 || f.DurationMs > 120_000 {
		t.Fatalf("duration %d", f.DurationMs)
	}
	if _, err := s.QuizAttemptFactsFor(ctx, uuid.NewString(), attemptID); err == nil {
		t.Fatal("another user must not read attempt facts")
	}
}
