// Command migrate applies SELAR's embedded SQL migrations as a separate deploy
// step. It records each migration with a checksum in schema_migrations, adopts
// databases initialised by Docker initdb, and never edits applied SQL.
//
// Use a direct (non-pooled) connection string: Neon's -pooler endpoint runs in
// transaction mode, which does not keep the session advisory lock.
//
//	MIGRATION_DATABASE_URL=postgres://...neon.tech/selar?sslmode=require go run ./cmd/migrate
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/dbconfig"
	"github.com/selar-dev/selar-api/internal/migrate"
)

func main() {
	status := flag.Bool("status", false, "print applied and pending migrations without changing the database")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall deadline")
	flag.Parse()

	databaseURL := os.Getenv("MIGRATION_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		log.Fatal("set MIGRATION_DATABASE_URL (direct connection) or DATABASE_URL")
	}
	if os.Getenv("MIGRATION_ALLOW_INSECURE") != "true" {
		if err := dbconfig.RequireTLSForRemote(databaseURL); err != nil {
			log.Fatal(err)
		}
	}

	files, err := migrate.LoadEmbedded()
	if err != nil {
		log.Fatal(err)
	}
	if len(files) == 0 {
		log.Fatal(migrate.ErrNoMigrations)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		log.Fatal("database URL could not be parsed (value hidden)")
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 15 * time.Second
	}
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		log.Fatalf("connect to %s failed", config.Host)
	}
	defer conn.Close(context.Background())

	if *status {
		pending, err := migrate.Status(ctx, conn, files)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("%d embedded migrations, %d pending", len(files), len(pending))
		for _, migration := range pending {
			log.Printf("pending %s", migration.Name)
		}
		return
	}

	result, err := migrate.Run(ctx, conn, files, log.Printf)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("migrations complete on %s: %d adopted, %d applied, %d total",
		config.Host, len(result.Adopted), len(result.Applied), len(files))
}
