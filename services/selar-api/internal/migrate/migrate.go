// Package migrate applies SELAR's ordered SQL migrations exactly once,
// records each one with a SHA-256 checksum in schema_migrations, refuses to
// run if an applied file was edited, and can adopt databases that Docker's
// initdb (or the README's manual psql steps) initialised before tracking existed.
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/store/migrations"
)

// Migration is one ordered SQL file.
type Migration struct {
	Version  string
	Name     string
	SQL      string
	Checksum string
}

var fileName = regexp.MustCompile(`^(\d{3,})_[a-z0-9_]+\.sql$`)

// Load reads *.sql files from dir, sorted by numeric version prefix.
func Load(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []Migration
	seen := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := fileName.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("migration %q must be named NNN_description.sql", entry.Name())
		}
		if previous, ok := seen[match[1]]; ok {
			return nil, fmt.Errorf("migrations %q and %q share version %s", previous, entry.Name(), match[1])
		}
		seen[match[1]] = entry.Name()
		data, err := fs.ReadFile(fsys, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		sum := sha256.Sum256(data)
		out = append(out, Migration{Version: match[1], Name: entry.Name(), SQL: string(data), Checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Version) != len(out[j].Version) {
			return len(out[i].Version) < len(out[j].Version)
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

// LoadEmbedded returns the migrations compiled into the binary.
func LoadEmbedded() ([]Migration, error) {
	return Load(migrations.FS, ".")
}

// Plan returns migrations that still need to run. Applied migrations must be
// an unedited, gap-free prefix of the files.
func Plan(files []Migration, applied map[string]string) ([]Migration, error) {
	known := map[string]bool{}
	for _, file := range files {
		known[file.Version] = true
	}
	for version := range applied {
		if !known[version] {
			return nil, fmt.Errorf("database has applied migration %s, which is not in this build; deploy a newer build", version)
		}
	}
	var pending []Migration
	for _, file := range files {
		checksum, ok := applied[file.Version]
		if !ok {
			pending = append(pending, file)
			continue
		}
		if len(pending) > 0 {
			return nil, fmt.Errorf("migration %s is applied but earlier migration %s is not", file.Version, pending[0].Version)
		}
		if checksum != file.Checksum {
			return nil, fmt.Errorf("checksum mismatch for applied migration %s: never edit applied SQL, add a new migration instead", file.Name)
		}
	}
	return pending, nil
}

// AdoptablePrefix returns how many leading versions have schema evidence.
// Evidence must be contiguous, otherwise the database state is ambiguous.
func AdoptablePrefix(versions []string, present map[string]bool) (int, error) {
	n := 0
	for n < len(versions) && present[versions[n]] {
		n++
	}
	for _, version := range versions[n:] {
		if present[version] {
			return 0, fmt.Errorf("cannot adopt: migration %s appears applied but an earlier migration is missing", version)
		}
	}
	return n, nil
}

// LastAdoptableVersion is the newest migration that existed before tracking.
// Newer migrations are always applied by the runner, never inferred.
const LastAdoptableVersion = "011"

// AdoptionProbes detect the final object each pre-tracking migration created
// in the current schema.
var AdoptionProbes = map[string]string{
	"001": tableProbe("quiz_responses"),
	"002": columnProbe("link_suggestions", "summary"),
	"003": indexProbe("idx_mental_models_embedding_hnsw"),
	"004": indexProbe("idx_learning_events_chat_message"),
	"005": tableProbe("chat_graph_updates"),
	"006": indexProbe("idx_evaluation_metrics_user_time"),
	"007": tableProbe("chunk_assets"),
	"008": columnProbe("chat_graph_updates", "concepts_created"),
	"009": indexProbe("idx_ingestion_jobs_document"),
	"010": columnProbe("link_suggestions", "evidence_verified"),
	"011": `SELECT EXISTS (SELECT 1 FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND t.tgname = 'reviewed_link_evidence_lock')`,
}

func tableProbe(table string) string {
	return fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = current_schema() AND tablename = '%s')`, table)
}

func columnProbe(table, column string) string {
	return fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = '%s' AND column_name = '%s')`, table, column)
}

func indexProbe(index string) string {
	return fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = '%s')`, index)
}

// Result summarises one runner invocation.
type Result struct {
	Adopted []string
	Applied []string
}

const lockKey = 7_265_646_172 // arbitrary constant: "selar" migrations

// Run applies pending migrations on a direct (non-pooled) connection.
// A session advisory lock serialises concurrent runners.
func Run(ctx context.Context, conn *pgx.Conn, files []Migration, logf func(string, ...any)) (Result, error) {
	var result Result
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return result, fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		checksum   TEXT NOT NULL,
		adopted    BOOLEAN NOT NULL DEFAULT false,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return result, fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := readApplied(ctx, conn)
	if err != nil {
		return result, err
	}
	if len(applied) == 0 {
		adopted, err := adopt(ctx, conn, files)
		if err != nil {
			return result, err
		}
		result.Adopted = adopted
		for _, version := range adopted {
			logf("adopted pre-existing migration %s", version)
		}
		if applied, err = readApplied(ctx, conn); err != nil {
			return result, err
		}
	}

	pending, err := Plan(files, applied)
	if err != nil {
		return result, err
	}
	for _, migration := range pending {
		if err := applyOne(ctx, conn, migration); err != nil {
			return result, err
		}
		result.Applied = append(result.Applied, migration.Version)
		logf("applied migration %s", migration.Name)
	}
	return result, nil
}

// Status reports pending migrations without writing anything. An untracked
// database reports every migration as pending (adoption happens in Run).
func Status(ctx context.Context, conn *pgx.Conn, files []Migration) ([]Migration, error) {
	var tracked bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_tables
		WHERE schemaname = current_schema() AND tablename = 'schema_migrations')`).Scan(&tracked); err != nil {
		return nil, err
	}
	if !tracked {
		return files, nil
	}
	applied, err := readApplied(ctx, conn)
	if err != nil {
		return nil, err
	}
	return Plan(files, applied)
}

func readApplied(ctx context.Context, conn *pgx.Conn) (map[string]string, error) {
	rows, err := conn.Query(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := map[string]string{}
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, err
		}
		applied[version] = checksum
	}
	return applied, rows.Err()
}

func adopt(ctx context.Context, conn *pgx.Conn, files []Migration) ([]string, error) {
	var versions []string
	present := map[string]bool{}
	for _, file := range files {
		if file.Version > LastAdoptableVersion {
			break
		}
		probe, ok := AdoptionProbes[file.Version]
		if !ok {
			return nil, fmt.Errorf("no adoption probe for migration %s", file.Version)
		}
		versions = append(versions, file.Version)
		var exists bool
		if err := conn.QueryRow(ctx, probe).Scan(&exists); err != nil {
			return nil, fmt.Errorf("probe migration %s: %w", file.Version, err)
		}
		present[file.Version] = exists
	}
	n, err := AdoptablePrefix(versions, present)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, file := range files[:n] {
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name, checksum, adopted) VALUES ($1, $2, $3, true)`,
			file.Version, file.Name, file.Checksum); err != nil {
			return nil, fmt.Errorf("record adopted migration %s: %w", file.Version, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return versions[:n], nil
}

func applyOne(ctx context.Context, conn *pgx.Conn, migration Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// No arguments: pgx sends the file with the simple protocol, so multiple
	// statements and dollar-quoted function bodies run as written.
	if _, err := tx.Exec(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.Name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		migration.Version, migration.Name, migration.Checksum); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.Name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.Name, err)
	}
	return nil
}

// ErrNoMigrations is returned when the embedded set is unexpectedly empty.
var ErrNoMigrations = errors.New("no migrations embedded in this build")
