// Command admin manages SELAR admin roles from a trusted machine.
//
//	DATABASE_URL='postgres://…?sslmode=require' go run ./cmd/admin promote someone@example.com
//	DATABASE_URL='…' go run ./cmd/admin demote someone@example.com
//	DATABASE_URL='…' go run ./cmd/admin list
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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/dbconfig"
	"github.com/selar-dev/selar-api/internal/store"
)

type roleStore interface {
	SetUserRoleByEmail(ctx context.Context, email, role string) (bool, error)
	ListAdminEmails(ctx context.Context) ([]string, error)
}

var errNoSuchUser = errors.New("no account with that email; register it in the console first")

const usage = "usage: admin promote <email> | admin demote <email> | admin list"

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
