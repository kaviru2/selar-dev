// Command admin manages SELAR admin roles from a trusted machine.
//
//	DATABASE_URL='postgres://…?sslmode=require' go run ./cmd/admin promote someone@example.com
//	DATABASE_URL='…' go run ./cmd/admin demote someone@example.com
//	DATABASE_URL='…' go run ./cmd/admin list
//	DATABASE_URL='…' go run ./cmd/admin lock <cohort> <setting> <json value> [reason…]
//	DATABASE_URL='…' go run ./cmd/admin unlock <cohort> <setting>
//	DATABASE_URL='…' go run ./cmd/admin locks
//
// Cohort locks pin a study-sensitive setting (see settings.Lockable, e.g.
// suggestions.show_on_open) for every member of a cohort; the Settings page
// shows it as fixed by the study and the API refuses to change it.
//
// The account must already exist (register through the console first). The
// change applies on the user's next admin request; no re-login is needed
// because the API reads the role from the database every time.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/mail"
	"os"
	"strings"
	"time"

	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/selar-dev/selar-api/internal/dbconfig"
	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/settings"
	"github.com/selar-dev/selar-api/internal/store"
)

type roleStore interface {
	SetUserRoleByEmail(ctx context.Context, email, role string) (bool, error)
	ListAdminEmails(ctx context.Context) ([]string, error)
	SetCohortSettingLock(ctx context.Context, cohort, key string, value any, reason string) error
	DeleteCohortSettingLock(ctx context.Context, cohort, key string) (bool, error)
	ListCohortSettingLocks(ctx context.Context) ([]store.CohortSettingLock, error)
}

var errNoSuchUser = errors.New("no account with that email; register it in the console first")

const usage = "usage: admin promote <email> | admin demote <email> | admin list | admin lock <cohort> <setting> <json value> [reason] | admin unlock <cohort> <setting> | admin locks"

func validCohort(c string) bool {
	switch model.Cohort(c) {
	case model.CohortControl, model.CohortTreatmentAuto, model.CohortTreatmentHITL:
		return true
	}
	return false
}

func run(ctx context.Context, args []string, roles roleStore, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "list":
		emails, err := roles.ListAdminEmails(ctx)
		if err != nil {
			return err
		}
		if len(emails) == 0 {
			fmt.Fprintln(out, "no admins")
		}
		for _, email := range emails {
			fmt.Fprintln(out, email)
		}
		return nil
	case "promote", "demote":
		if len(args) != 2 {
			return errors.New(usage)
		}
		email := strings.TrimSpace(args[1])
		if parsed, err := mail.ParseAddress(email); err != nil || parsed.Address != email {
			return fmt.Errorf("%q is not a valid email address", email)
		}
		role, verb := "admin", "promoted"
		if args[0] == "demote" {
			role, verb = "user", "demoted"
		}
		found, err := roles.SetUserRoleByEmail(ctx, email, role)
		if err != nil {
			return err
		}
		if !found {
			return errNoSuchUser
		}
		fmt.Fprintf(out, "%s %s to %s\n", verb, email, role)
		return nil
	case "lock":
		if len(args) < 4 {
			return errors.New(usage)
		}
		cohort, key := args[1], args[2]
		if !validCohort(cohort) {
			return fmt.Errorf("unknown cohort %q", cohort)
		}
		if !settings.Lockable(key) {
			return fmt.Errorf("%q cannot be locked per cohort", key)
		}
		var raw any
		if err := json.Unmarshal([]byte(args[3]), &raw); err != nil {
			return fmt.Errorf("value must be JSON (for example false or \"dark\"): %w", err)
		}
		value, err := settings.Validate(key, raw)
		if err != nil {
			return err
		}
		reason := strings.Join(args[4:], " ")
		if err := roles.SetCohortSettingLock(ctx, cohort, key, value, reason); err != nil {
			return err
		}
		fmt.Fprintf(out, "locked %s for %s to %v\n", key, cohort, value)
		return nil
	case "unlock":
		if len(args) != 3 {
			return errors.New(usage)
		}
		found, err := roles.DeleteCohortSettingLock(ctx, args[1], args[2])
		if err != nil {
			return err
		}
		if !found {
			fmt.Fprintln(out, "no such lock")
			return nil
		}
		fmt.Fprintf(out, "unlocked %s for %s\n", args[2], args[1])
		return nil
	case "locks":
		locks, err := roles.ListCohortSettingLocks(ctx)
		if err != nil {
			return err
		}
		if len(locks) == 0 {
			fmt.Fprintln(out, "no cohort locks")
		}
		for _, l := range locks {
			v, _ := json.Marshal(l.Value)
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", l.Cohort, l.Key, v, l.Reason)
		}
		return nil
	}
	return errors.New(usage)
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("set DATABASE_URL")
	}
	if err := dbconfig.RequireTLSForRemote(databaseURL); err != nil && os.Getenv("ADMIN_ALLOW_INSECURE") != "true" {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal("database URL could not be used (value hidden)")
	}
	defer pool.Close()
	if err := run(ctx, os.Args[1:], store.New(pool), os.Stdout); err != nil {
		log.Fatal(err)
	}
}
