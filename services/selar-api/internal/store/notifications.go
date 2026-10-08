package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/notify"
)

// NotificationRecipient returns one learner's email opt-ins.
func (s *Store) NotificationRecipient(ctx context.Context, userID string) (notify.Recipient, error) {
	r := notify.Recipient{UserID: userID}
	err := s.pool.QueryRow(ctx, `SELECT email, email_quiz_notices, email_security_notices, email_unsubscribe_version
		FROM users WHERE id = $1`, userID).Scan(&r.Email, &r.QuizEmails, &r.SecurityEmails, &r.UnsubscribeVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrUserNotFound
	}
	return r, err
}

// QuizNotificationRecipients lists learners who opted in to quiz emails.
func (s *Store) QuizNotificationRecipients(ctx context.Context) ([]notify.Recipient, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, email, email_quiz_notices, email_security_notices, email_unsubscribe_version
		FROM users WHERE email_quiz_notices ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []notify.Recipient
	for rows.Next() {
		var r notify.Recipient
		if err := rows.Scan(&r.UserID, &r.Email, &r.QuizEmails, &r.SecurityEmails, &r.UnsubscribeVersion); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetNotificationPrefs updates the opt-ins; nil leaves a value unchanged.
func (s *Store) SetNotificationPrefs(ctx context.Context, userID string, quizEmails, securityEmails *bool) (notify.Recipient, error) {
	r := notify.Recipient{UserID: userID}
	err := s.pool.QueryRow(ctx, `UPDATE users SET
		email_quiz_notices = COALESCE($2, email_quiz_notices),
		email_security_notices = COALESCE($3, email_security_notices)
		WHERE id = $1 RETURNING email, email_quiz_notices, email_security_notices, email_unsubscribe_version`,
		userID, quizEmails, securityEmails).Scan(&r.Email, &r.QuizEmails, &r.SecurityEmails, &r.UnsubscribeVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrUserNotFound
	}
	return r, err
}

// UnsubscribeAll turns every email notice off if version is current, and
// invalidates the link. It reports whether anything changed.
func (s *Store) UnsubscribeAll(ctx context.Context, userID string, version int) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET email_quiz_notices = false, email_security_notices = false,
		email_unsubscribe_version = email_unsubscribe_version + 1
		WHERE id = $1 AND email_unsubscribe_version = $2`, userID, version)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ClaimNotification inserts a pending audit row unless the dedupe key exists.
func (s *Store) ClaimNotification(ctx context.Context, c notify.Claim) (int64, bool, error) {
	var quizID any
	if c.QuizID != "" {
		quizID = c.QuizID
	}
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO notification_log
		(user_id, kind, dedupe_key, quiz_id, window_start, window_end, recipient_hash, transport, subject)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (dedupe_key) DO NOTHING RETURNING id`,
		c.UserID, string(c.Kind), c.DedupeKey, quizID, c.WindowStart, c.WindowEnd, c.RecipientHash, c.Transport, c.Subject).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// FinishNotification records the send outcome.
func (s *Store) FinishNotification(ctx context.Context, id int64, status, errText string) error {
	_, err := s.pool.Exec(ctx, `UPDATE notification_log SET status = $2, error = $3, finished_at = now() WHERE id = $1`, id, status, errText)
	return err
}

// CountNotificationsSince counts notices that were sent, recorded as
// dry-run or are still pending since t. userID "" counts every learner.
func (s *Store) CountNotificationsSince(ctx context.Context, userID string, since time.Time) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM notification_log
		WHERE created_at >= $1 AND status <> 'failed' AND ($2 = '' OR user_id::text = $2)`, since, userID).Scan(&n)
	return n, err
}

// NotificationLogEntry is an audit row as shown to admins (no addresses).
type NotificationLogEntry struct {
	ID          int64      `json:"id"`
	UserID      string     `json:"user_id"`
	Kind        string     `json:"kind"`
	QuizID      *string    `json:"quiz_id,omitempty"`
	WindowStart *time.Time `json:"window_start,omitempty"`
	WindowEnd   *time.Time `json:"window_end,omitempty"`
	Transport   string     `json:"transport"`
	Subject     string     `json:"subject"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ListNotificationLog returns the newest audit rows.
func (s *Store) ListNotificationLog(ctx context.Context, limit int) ([]NotificationLogEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT id, user_id::text, kind, quiz_id::text, window_start, window_end, transport, subject, status, error, created_at
		FROM notification_log ORDER BY created_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotificationLogEntry{}
	for rows.Next() {
		var e NotificationLogEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Kind, &e.QuizID, &e.WindowStart, &e.WindowEnd, &e.Transport, &e.Subject, &e.Status, &e.Error, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
