package migrate

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadOrdersByNumericVersionAndChecksumsContent(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/010_ten.sql":  {Data: []byte("SELECT 10;")},
		"migrations/002_two.sql":  {Data: []byte("SELECT 2;")},
		"migrations/001_init.sql": {Data: []byte("SELECT 1;")},
		"migrations/README.md":    {Data: []byte("ignored")},
	}
	got, err := Load(fsys, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d migrations, want 3", len(got))
	}
	want := []string{"001", "002", "010"}
	for i, migration := range got {
		if migration.Version != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
		if len(migration.Checksum) != 64 {
			t.Fatalf("checksum must be hex SHA-256, got %q", migration.Checksum)
		}
	}
	if got[0].Name != "001_init.sql" || got[0].SQL != "SELECT 1;" {
		t.Fatalf("unexpected first migration: %+v", got[0])
	}
	if got[0].Checksum == got[1].Checksum {
		t.Fatal("different SQL must have different checksums")
	}
}

func TestLoadRejectsDuplicateAndMalformedVersions(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"duplicate": {
			"m/001_a.sql": {Data: []byte("SELECT 1;")},
			"m/001_b.sql": {Data: []byte("SELECT 1;")},
		},
		"malformed": {"m/init.sql": {Data: []byte("SELECT 1;")}},
		"empty":     {"m/001_empty.sql": {Data: []byte("  \n")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(fsys, "m"); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestPlanRejectsEditedOrUnknownAppliedMigrations(t *testing.T) {
	files := []Migration{
		{Version: "001", Name: "001_a.sql", Checksum: "aaa"},
		{Version: "002", Name: "002_b.sql", Checksum: "bbb"},
	}
	pending, err := Plan(files, map[string]string{"001": "aaa"})
	if err != nil || len(pending) != 1 || pending[0].Version != "002" {
		t.Fatalf("pending = %+v, err = %v", pending, err)
	}
	if _, err := Plan(files, map[string]string{"001": "changed"}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("edited applied migration must fail with checksum error, got %v", err)
	}
	if _, err := Plan(files, map[string]string{"001": "aaa", "003": "ccc"}); err == nil {
		t.Fatal("an applied migration missing from the files must fail")
	}
	if _, err := Plan(files, map[string]string{"002": "bbb"}); err == nil {
		t.Fatal("a gap before an applied migration must fail")
	}
}

func TestAdoptablePrefixRequiresContiguousEvidence(t *testing.T) {
	versions := []string{"001", "002", "003", "004"}
	n, err := AdoptablePrefix(versions, map[string]bool{"001": true, "002": true})
	if err != nil || n != 2 {
		t.Fatalf("prefix = %d, err = %v", n, err)
	}
	n, err = AdoptablePrefix(versions, map[string]bool{})
	if err != nil || n != 0 {
		t.Fatalf("empty database prefix = %d, err = %v", n, err)
	}
	if _, err := AdoptablePrefix(versions, map[string]bool{"001": true, "003": true}); err == nil {
		t.Fatal("non-contiguous schema evidence must refuse adoption")
	}
}

func TestEveryEmbeddedMigrationHasAnAdoptionProbe(t *testing.T) {
	migrations, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 11 {
		t.Fatalf("expected the repository migrations, got %d", len(migrations))
	}
	for _, migration := range migrations {
		if _, ok := AdoptionProbes[migration.Version]; !ok && migration.Version <= LastAdoptableVersion {
			t.Errorf("migration %s has no adoption probe", migration.Name)
		}
	}
}
