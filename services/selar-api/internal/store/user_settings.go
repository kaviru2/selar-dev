package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrEmailTaken is returned when another account already uses the address.
var ErrEmailTaken = errors.New("email already in use")

// ErrUserNotFound is returned when the user row does not exist.
var ErrUserNotFound = errors.New("user not found")

// GetUserSettings returns the stored preference document, the user's cohort
// and every lock configured for that cohort.
func (s *Store) GetUserSettings(ctx context.Context, userID string) (map[string]any, string, map[string]any, error) {
	var raw []byte
	var cohort string
	err := s.pool.QueryRow(ctx, `SELECT preferences, cohort FROM users WHERE id = $1`, userID).Scan(&raw, &cohort)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil, ErrUserNotFound
	}
	if err != nil {
		return nil, "", nil, err
	}
	stored := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &stored)
	}
	rows, err := s.pool.Query(ctx, `SELECT setting_key, value FROM cohort_setting_locks WHERE cohort = $1`, cohort)
	if err != nil {
		return nil, "", nil, err
	}
	defer rows.Close()
	locks := map[string]any{}
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, "", nil, err
		}
		var v any
		if json.Unmarshal(value, &v) == nil {
			locks[key] = v
		}
	}
	return stored, cohort, locks, rows.Err()
}

// MergeUserSettings shallow-merges an already validated patch into the
// stored preferences, so saving one setting never erases another.
func (s *Store) MergeUserSettings(ctx context.Context, userID string, patch map[string]any) error {
	raw, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET preferences = preferences || $1::jsonb WHERE id = $2`, raw, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SetCohortSettingLock pins a setting for every member of a cohort. Callers
// must check settings.Lockable first.
func (s *Store) SetCohortSettingLock(ctx context.Context, cohort, key string, value any, reason string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO cohort_setting_locks (cohort, setting_key, value, reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (cohort, setting_key) DO UPDATE SET value = EXCLUDED.value, reason = EXCLUDED.reason, updated_at = now()`,
		cohort, key, raw, reason)
	return err
}

// GetUserAuth returns the email, password hash and session version.
func (s *Store) GetUserAuth(ctx context.Context, userID string) (string, string, int, error) {
	var email, hash string
	var version int
	err := s.pool.QueryRow(ctx, `SELECT email, password_hash, session_version FROM users WHERE id = $1`, userID).
		Scan(&email, &hash, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", 0, ErrUserNotFound
	}
	return email, hash, version, err
}

// SessionVersion returns the user's current session version. Tokens carrying
// an older version are rejected by the auth middleware.
func (s *Store) SessionVersion(ctx context.Context, userID string) (int, error) {
	var version int
	err := s.pool.QueryRow(ctx, `SELECT session_version FROM users WHERE id = $1`, userID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrUserNotFound
	}
	return version, err
}

// UpdateDisplayName stores an already normalised display name.
func (s *Store) UpdateDisplayName(ctx context.Context, userID, name string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET display_name = $1 WHERE id = $2`, name, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateEmail changes the sign-in address. Addresses are compared without
// regard to case so two accounts cannot differ only by capitalisation.
func (s *Store) UpdateEmail(ctx context.Context, userID, email string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var taken bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1) AND id <> $2)`,
		email, userID).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return ErrEmailTaken
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET email = $1 WHERE id = $2`, email, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEmailTaken
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return tx.Commit(ctx)
}

// UpdatePassword stores a new bcrypt hash and increments the session
// version, which ends every session issued under the old version. It returns
// the new version so the caller can issue a fresh token for this session.
func (s *Store) UpdatePassword(ctx context.Context, userID, hash string) (int, error) {
	var version int
	err := s.pool.QueryRow(ctx, `UPDATE users SET password_hash = $1, session_version = session_version + 1
		WHERE id = $2 RETURNING session_version`, hash, userID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrUserNotFound
	}
	return version, err
}

// CohortSettingLock is one administrator-pinned setting.
type CohortSettingLock struct {
	Cohort    string
	Key       string
	Value     any
	Reason    string
	UpdatedAt time.Time
}

// ListCohortSettingLocks returns every lock, ordered by cohort and key.
func (s *Store) ListCohortSettingLocks(ctx context.Context) ([]CohortSettingLock, error) {
	rows, err := s.pool.Query(ctx, `SELECT cohort, setting_key, value, COALESCE(reason, ''), updated_at
		FROM cohort_setting_locks ORDER BY cohort, setting_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CohortSettingLock
	for rows.Next() {
		var l CohortSettingLock
		var raw []byte
		if err := rows.Scan(&l.Cohort, &l.Key, &raw, &l.Reason, &l.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &l.Value)
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeleteCohortSettingLock removes a lock; false when there was none.
func (s *Store) DeleteCohortSettingLock(ctx context.Context, cohort, key string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM cohort_setting_locks WHERE cohort = $1 AND setting_key = $2`, cohort, key)
	return tag.RowsAffected() > 0, err
}
