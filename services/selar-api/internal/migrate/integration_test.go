package migrate

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// isolatedConn opens TEST_DATABASE_URL with search_path pinned to a fresh
// schema so each scenario starts from an empty database view.
func isolatedConn(t *testing.T) *pgx.Conn {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("migrate_it_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close(context.Background())
	})
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["search_path"] = schema + ",public"
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func noLog(string, ...any) {}

func TestIntegrationRunAppliesAllThenIsIdempotent(t *testing.T) {
	conn := isolatedConn(t)
	ctx := context.Background()
	files, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	first, err := Run(ctx, conn, files, noLog)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Applied) != len(files) || len(first.Adopted) != 0 {
		t.Fatalf("fresh database: applied %v adopted %v", first.Applied, first.Adopted)
	}
	second, err := Run(ctx, conn, files, noLog)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Applied) != 0 || len(second.Adopted) != 0 {
		t.Fatalf("second run must be a no-op, got %+v", second)
	}
	var vectorType bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')`).Scan(&vectorType); err != nil || !vectorType {
		t.Fatalf("pgvector extension must exist after migrations: %v", err)
	}
}

func TestIntegrationAdoptsDockerInitdbDatabaseAndAppliesTheRest(t *testing.T) {
	conn := isolatedConn(t)
	ctx := context.Background()
	files, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	// docker-compose initdb only mounted 001-009, without any tracking table.
	for _, file := range files {
		if file.Version > "009" {
			break
		}
		if _, err := conn.Exec(ctx, file.SQL); err != nil {
			t.Fatalf("raw apply %s: %v", file.Name, err)
		}
	}
	result, err := Run(ctx, conn, files, noLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.Adopted, ",") != "001,002,003,004,005,006,007,008,009" {
		t.Fatalf("adopted = %v", result.Adopted)
	}
	if strings.Join(result.Applied, ",") != "010,011" {
		t.Fatalf("applied = %v", result.Applied)
	}
	var adoptedRows int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE adopted`).Scan(&adoptedRows); err != nil || adoptedRows != 9 {
		t.Fatalf("adopted rows = %d, err = %v", adoptedRows, err)
	}
}

func TestIntegrationStatusIsReadOnly(t *testing.T) {
	conn := isolatedConn(t)
	ctx := context.Background()
	files, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := Status(ctx, conn, files)
	if err != nil || len(pending) != len(files) {
		t.Fatalf("untracked empty database: pending %d, err %v", len(pending), err)
	}
	var tracked bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_tables
		WHERE schemaname = current_schema() AND tablename = 'schema_migrations')`).Scan(&tracked); err != nil || tracked {
		t.Fatalf("status must not create schema_migrations (tracked=%v err=%v)", tracked, err)
	}
	if _, err := Run(ctx, conn, files, noLog); err != nil {
		t.Fatal(err)
	}
	pending, err = Status(ctx, conn, files)
	if err != nil || len(pending) != 0 {
		t.Fatalf("after run: pending %d, err %v", len(pending), err)
	}
}

func TestIntegrationRefusesEditedAppliedMigration(t *testing.T) {
	conn := isolatedConn(t)
	ctx := context.Background()
	files, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, conn, files, noLog); err != nil {
		t.Fatal(err)
	}
	edited := append([]Migration(nil), files...)
	edited[1].Checksum = strings.Repeat("0", 64)
	if _, err := Run(ctx, conn, edited, noLog); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestIntegrationRefusesAmbiguousPartialSchema(t *testing.T) {
	conn := isolatedConn(t)
	ctx := context.Background()
	files, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	// Evidence for 002 without 001 is not a state any supported path produces.
	if _, err := conn.Exec(ctx, `CREATE TABLE link_suggestions (summary TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, conn, files, noLog); err == nil || !strings.Contains(err.Error(), "cannot adopt") {
		t.Fatalf("expected adoption refusal, got %v", err)
	}
}
