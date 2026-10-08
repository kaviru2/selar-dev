package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// settingsFixture creates two synthetic users that are removed afterwards.
func settingsFixture(t *testing.T) (*Store, context.Context, string, string) {
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
	var a, b string
	for _, id := range []*string{&a, &b} {
		if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,cohort)
			VALUES ('selar.qa+'||gen_random_uuid()::text||'@example.com','x','control') RETURNING id`).Scan(id); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1 OR id = $2`, a, b)
		_, _ = pool.Exec(ctx, `DELETE FROM cohort_setting_locks WHERE setting_key LIKE 'test.%'`)
		pool.Close()
	})
	return New(pool), ctx, a, b
}

func TestAccountMergeUserSettingsKeepsOtherKeys(t *testing.T) {
	s, ctx, a, _ := settingsFixture(t)
	if err := s.MergeUserSettings(ctx, a, map[string]any{"theme": "dark"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeUserSettings(ctx, a, map[string]any{"reader": map[string]any{"defaultZoom": 1.2}}); err != nil {
		t.Fatal(err)
	}
	stored, cohort, _, err := s.GetUserSettings(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if stored["theme"] != "dark" || stored["reader"].(map[string]any)["defaultZoom"] != 1.2 || cohort != "control" {
		t.Fatalf("merge lost a key: %#v cohort=%s", stored, cohort)
	}
}

func TestAccountCohortLocksAreScopedToCohort(t *testing.T) {
	s, ctx, a, _ := settingsFixture(t)
	if err := s.SetCohortSettingLock(ctx, "control", "test.key", true, "synthetic"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCohortSettingLock(ctx, "treatment_hitl", "test.other", false, "synthetic"); err != nil {
		t.Fatal(err)
	}
	_, _, locks, err := s.GetUserSettings(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if locks["test.key"] != true {
		t.Fatalf("control lock missing: %#v", locks)
	}
	if _, ok := locks["test.other"]; ok {
		t.Fatal("another cohort's lock leaked")
	}
}

func TestAccountUpdateEmailRejectsTakenAddressCaseInsensitively(t *testing.T) {
	s, ctx, a, b := settingsFixture(t)
	other, _, _, err := s.GetUserAuth(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	upper := []byte(other)
	for i, c := range upper {
		if c >= 'a' && c <= 'z' {
			upper[i] = c - 32
		}
	}
	if err := s.UpdateEmail(ctx, a, string(upper)); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("taken email: got %v, want ErrEmailTaken", err)
	}
	if err := s.UpdateEmail(ctx, a, "selar.qa+renamed-"+a+"@example.com"); err != nil {
		t.Fatal(err)
	}
	email, _, _, _ := s.GetUserAuth(ctx, a)
	if email != "selar.qa+renamed-"+a+"@example.com" {
		t.Fatalf("email not updated: %s", email)
	}
}

func TestAccountUpdatePasswordBumpsSessionVersion(t *testing.T) {
	s, ctx, a, b := settingsFixture(t)
	_, _, before, _ := s.GetUserAuth(ctx, a)
	version, err := s.UpdatePassword(ctx, a, "new-hash")
	if err != nil {
		t.Fatal(err)
	}
	if version != before+1 {
		t.Fatalf("session version %d, want %d", version, before+1)
	}
	_, hash, after, _ := s.GetUserAuth(ctx, a)
	if hash != "new-hash" || after != version {
		t.Fatalf("hash=%q version=%d", hash, after)
	}
	got, err := s.SessionVersion(ctx, a)
	if err != nil || got != version {
		t.Fatalf("SessionVersion = %d, %v", got, err)
	}
	if other, _ := s.SessionVersion(ctx, b); other != 0 {
		t.Fatalf("other user's sessions must be untouched, got %d", other)
	}
}

func TestAccountDisplayNameRoundTrip(t *testing.T) {
	s, ctx, a, _ := settingsFixture(t)
	if err := s.UpdateDisplayName(ctx, a, "Synthetic Tester"); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUserByID(ctx, a)
	if err != nil || u.DisplayName != "Synthetic Tester" {
		t.Fatalf("display name = %q, %v", u.DisplayName, err)
	}
}

func TestAccountListAndDeleteCohortSettingLocks(t *testing.T) {
	s, ctx, _, _ := settingsFixture(t)
	if err := s.SetCohortSettingLock(ctx, "treatment_auto", "test.list", "x", "synthetic"); err != nil {
		t.Fatal(err)
	}
	locks, err := s.ListCohortSettingLocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range locks {
		if l.Cohort == "treatment_auto" && l.Key == "test.list" && l.Value == "x" && l.Reason == "synthetic" {
			found = true
		}
	}
	if !found {
		t.Fatalf("lock not listed: %#v", locks)
	}
	if ok, err := s.DeleteCohortSettingLock(ctx, "treatment_auto", "test.list"); err != nil || !ok {
		t.Fatalf("delete: %v %v", ok, err)
	}
	if ok, _ := s.DeleteCohortSettingLock(ctx, "treatment_auto", "test.list"); ok {
		t.Fatal("second delete must report no lock")
	}
}
