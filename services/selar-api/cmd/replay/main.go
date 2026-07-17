// Command replay deterministically verifies or rebuilds one user's adaptive
// graph and learner projections from retained evidence and learning events.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/store"
)

func main() {
	userID := flag.String("user", "", "user UUID to replay")
	apply := flag.Bool("apply", false, "apply the rebuilt projections")
	asOfValue := flag.String("as-of", "", "optional RFC3339 evaluation time")
	flag.Parse()
	if *userID == "" {
		log.Fatal("-user is required")
	}
	asOf := time.Now().UTC()
	if *asOfValue != "" {
		parsed, err := time.Parse(time.RFC3339, *asOfValue)
		if err != nil {
			log.Fatalf("invalid -as-of value: %v", err)
		}
		asOf = parsed
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://selar:selar_dev@localhost:5432/selar?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	report, err := store.New(pool).ReplayAdaptiveGraph(ctx, *userID, *apply, asOf)
	if err != nil {
		log.Fatal(err)
	}
	output, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(output))
}
