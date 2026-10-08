package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/selar-dev/selar-api/internal/model"
)

// ErrGoogleAlreadyLinked is returned when an account is already linked to a
// different Google identity.
var ErrGoogleAlreadyLinked = errors.New("account is linked to a different Google account")

// GoogleAccount is a user together with the Google identity linked to it
// (empty when none).
type GoogleAccount struct {
	User      *model.User
	GoogleSub string
}

// GetUserByGoogleSub returns the account linked to a Google subject, or
// ErrUserNotFound.
func (s *Store) GetUserByGoogleSub(ctx context.Context, sub string) (*model.User, error) {
	u := &model.User{}
	err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE google_sub = $1`, sub), u)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return u, err
}

// GetGoogleAccountByEmail returns the account whose email equals email
// (compared case-insensitively, like UpdateEmail's uniqueness check), or
// ErrUserNotFound. More than one case-variant match is treated as not found
// so a Google sign-in never picks between two accounts.
func (s *Store) GetGoogleAccountByEmail(ctx context.Context, email string) (*GoogleAccount, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+userColumns+`, COALESCE(google_sub, '') FROM users WHERE lower(email) = lower($1) LIMIT 2`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []*GoogleAccount
	for rows.Next() {
		acc := &GoogleAccount{User: &model.User{}}
		if err := scanUser(rows, acc.User, &acc.GoogleSub); err != nil {
			return nil, err
		}
		found = append(found, acc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(found) != 1 {
		return nil, ErrUserNotFound
	}
	return found[0], nil
}

// LinkGoogleSub links a Google subject to an account that has none yet and
// increments the session version, so sessions opened before the link (for
// example by someone who registered the address first) end. It returns the
// new session version.
func (s *Store) LinkGoogleSub(ctx context.Context, userID, sub string) (int, error) {
	var version int
	err := s.pool.QueryRow(ctx,
		`UPDATE users SET google_sub = $2, session_version = session_version + 1
		 WHERE id = $1 AND google_sub IS NULL RETURNING session_version`, userID, sub).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrGoogleAlreadyLinked
	}
	return version, err
}
