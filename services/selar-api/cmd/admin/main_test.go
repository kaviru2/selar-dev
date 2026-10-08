package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRoles struct {
	emails map[string]string
	admins []string
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
