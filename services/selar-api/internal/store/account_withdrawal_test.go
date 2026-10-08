package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAccountDeletionWritesAnonymousWithdrawalRecord(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	email := "withdrawal-store-" + uuid.NewString() + "@example.test"
	groupLabel := "withdrawal-store-" + uuid.NewString()
	createdAt := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,cohort,group_label,created_at)
		VALUES ($1,'synthetic','treatment_hitl',$2,$3) RETURNING id`, email, groupLabel, createdAt).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	var withdrawalID int64
	t.Cleanup(func() {
		if withdrawalID != 0 {
			_, _ = pool.Exec(context.Background(), `DELETE FROM study_withdrawals WHERE id=$1`, withdrawalID)
		}
	})

	if err := New(pool).DeleteUser(ctx, userID); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM study_withdrawals WHERE group_label=$1`, groupLabel).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("withdrawal rows for synthetic user = %d, want exactly 1", count)
	}

	var cohort, gotGroup, reason, rowJSON string
	var gotCreatedAt, withdrawnAt time.Time
	if err := pool.QueryRow(ctx, `SELECT id, cohort, group_label, account_created_at, withdrawn_at, reason,
		to_jsonb(study_withdrawals)::text
		FROM study_withdrawals WHERE group_label=$1`, groupLabel).
		Scan(&withdrawalID, &cohort, &gotGroup, &gotCreatedAt, &withdrawnAt, &reason, &rowJSON); err != nil {
		t.Fatal(err)
	}
	if cohort != "treatment_hitl" || gotGroup != groupLabel {
		t.Fatalf("withdrawal metadata = cohort %q group %q", cohort, gotGroup)
	}
	if reason != "account_deleted" {
		t.Fatalf("reason = %q, want account_deleted", reason)
	}
	if !gotCreatedAt.Equal(createdAt) {
		t.Fatalf("account_created_at = %s, want %s", gotCreatedAt, createdAt)
	}
	if withdrawnAt.IsZero() {
		t.Fatal("withdrawn_at was not populated")
	}
	if strings.Contains(rowJSON, email) || strings.Contains(rowJSON, userID) {
		t.Fatalf("anonymous withdrawal row contains an identifier: %s", rowJSON)
	}
}
