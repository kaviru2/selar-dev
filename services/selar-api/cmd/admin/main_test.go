package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/selar-dev/selar-api/internal/store"
)

type fakeRoles struct {
	emails   map[string]string
	admins   []string
	locks    []lockCall
	unlocked []string
}

func (f *fakeRoles) SetUserRoleByEmail(_ context.Context, email, role string) (bool, error) {
	key := strings.ToLower(strings.TrimSpace(email))
	if _, ok := f.emails[key]; !ok {
		return false, nil
	}
	f.emails[key] = role
	return true, nil
}

func (f *fakeRoles) ListAdminEmails(context.Context) ([]string, error) { return f.admins, nil }

func TestRunPromoteAndDemote(t *testing.T) {
	roles := &fakeRoles{emails: map[string]string{"a@example.com": "user"}}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"promote", "A@example.com"}, roles, &out); err != nil {
		t.Fatal(err)
	}
	if roles.emails["a@example.com"] != "admin" || !strings.Contains(out.String(), "promoted") {
		t.Fatalf("promote failed: %v %q", roles.emails, out.String())
	}
	if err := run(context.Background(), []string{"demote", "a@example.com"}, roles, &out); err != nil || roles.emails["a@example.com"] != "user" {
		t.Fatalf("demote failed: %v %v", roles.emails, err)
	}
}

func TestRunRejectsUnknownEmailAndBadUsage(t *testing.T) {
	roles := &fakeRoles{emails: map[string]string{}}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"promote", "nobody@example.com"}, roles, &out); !errors.Is(err, errNoSuchUser) {
		t.Fatalf("unknown email must fail with errNoSuchUser, got %v", err)
	}
	for _, args := range [][]string{{}, {"promote"}, {"delete", "a@example.com"}, {"promote", "not-an-email"}} {
		if err := run(context.Background(), args, roles, &out); err == nil {
			t.Fatalf("args %v must be rejected", args)
		}
	}
}

func TestRunListPrintsAdmins(t *testing.T) {
	roles := &fakeRoles{admins: []string{"x@example.com"}}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"list"}, roles, &out); err != nil || !strings.Contains(out.String(), "x@example.com") {
		t.Fatalf("list: %v %q", err, out.String())
	}
}

type lockCall struct {
	cohort, key string
	value       any
	reason      string
}

func (f *fakeRoles) SetCohortSettingLock(_ context.Context, cohort, key string, value any, reason string) error {
	f.locks = append(f.locks, lockCall{cohort, key, value, reason})
	return nil
}

func (f *fakeRoles) DeleteCohortSettingLock(_ context.Context, cohort, key string) (bool, error) {
	f.unlocked = append(f.unlocked, cohort+"/"+key)
	return true, nil
}

func (f *fakeRoles) ListCohortSettingLocks(context.Context) ([]store.CohortSettingLock, error) {
	return []store.CohortSettingLock{{Cohort: "control", Key: "suggestions.show_on_open", Value: false, Reason: "protocol v1"}}, nil
}

func TestRunLockValidatesCohortKeyAndValue(t *testing.T) {
	f := &fakeRoles{}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"lock", "control", "suggestions.show_on_open", "false", "protocol", "v1"}, f, &out); err != nil {
		t.Fatal(err)
	}
	if len(f.locks) != 1 || f.locks[0].value != false || f.locks[0].reason != "protocol v1" || f.locks[0].cohort != "control" {
		t.Fatalf("locks = %#v", f.locks)
	}
	for _, args := range [][]string{
		{"lock", "control", "suggestions.show_on_open"},          // no value
		{"lock", "nobody", "suggestions.show_on_open", "false"},  // unknown cohort
		{"lock", "control", "reader.zoom", "1.2"},                // not lockable
		{"lock", "control", "suggestions.show_on_open", "maybe"}, // invalid value
		{"unlock", "control"},
	} {
		if err := run(context.Background(), args, f, &out); err == nil {
			t.Fatalf("args %v must be rejected", args)
		}
	}
	if len(f.locks) != 1 {
		t.Fatalf("rejected commands must not write: %#v", f.locks)
	}
}

func TestRunUnlockAndListLocks(t *testing.T) {
	f := &fakeRoles{}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"unlock", "control", "suggestions.show_on_open"}, f, &out); err != nil || len(f.unlocked) != 1 {
		t.Fatalf("unlock: %v %v", err, f.unlocked)
	}
	out.Reset()
	if err := run(context.Background(), []string{"locks"}, f, &out); err != nil || !strings.Contains(out.String(), "control\tsuggestions.show_on_open\tfalse") {
		t.Fatalf("locks: %v %q", err, out.String())
	}
}
