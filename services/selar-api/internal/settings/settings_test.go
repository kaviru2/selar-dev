package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestValidatePatchAcceptsKnownKeys(t *testing.T) {
	got, err := ValidatePatch(decode(t, `{"colorScheme":"dark","theme":"warm","reader.zoom":1.25,"reader.page_fit":"width","reader.highlight_color":"green","suggestions.show_on_open":false}`))
	if err != nil {
		t.Fatalf("valid patch rejected: %v", err)
	}
	if got["colorScheme"] != "dark" || got["theme"] != "warm" || got["reader.zoom"] != 1.25 || got["reader.page_fit"] != "width" ||
		got["reader.highlight_color"] != "green" || got["suggestions.show_on_open"] != false {
		t.Fatalf("unexpected normalised patch: %#v", got)
	}
}

func TestValidatePatchRejectsUnknownAndInvalidValues(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"density":"compact"}`,
		`{"colorScheme":"neon"}`,
		`{"colorScheme":3}`,
		`{"theme":"neon"}`,
		`{"reader.zoom":5}`,
		`{"reader.zoom":0.1}`,
		`{"reader.zoom":"1"}`,
		`{"reader.page_fit":"stretch"}`,
		`{"reader.highlight_color":"#ff0000"}`,
		`{"suggestions.show_on_open":"no"}`,
		`{"cohort":"control"}`,
	} {
		if _, err := ValidatePatch(decode(t, raw)); err == nil {
			t.Errorf("patch %s should be rejected", raw)
		}
	}
}

func TestValidatePatchRoundsZoomToWholePercent(t *testing.T) {
	got, err := ValidatePatch(decode(t, `{"reader.zoom":1.2345}`))
	if err != nil || got["reader.zoom"] != 1.23 {
		t.Fatalf("zoom = %#v, %v", got["reader.zoom"], err)
	}
}

func TestEffectiveAppliesDefaultsDropsUnknownAndLocksWin(t *testing.T) {
	stored := map[string]any{"colorScheme": "dark", "density": "compact", "reader.zoom": 9.0, "suggestions.show_on_open": false}
	locks := map[string]any{"suggestions.show_on_open": true}
	values, locked := Effective(stored, locks)
	if values["colorScheme"] != "dark" || values["theme"] != "paper" {
		t.Fatalf("stored colour scheme lost: %#v", values)
	}
	if _, ok := values["density"]; ok {
		t.Fatal("unknown legacy key must not be exposed")
	}
	if values["reader.zoom"] != Defaults()["reader.zoom"] {
		t.Fatalf("invalid stored zoom must fall back to the default, got %#v", values["reader.zoom"])
	}
	if values["suggestions.show_on_open"] != true {
		t.Fatal("cohort lock must override the learner's stored value")
	}
	if len(locked) != 1 || locked[0] != "suggestions.show_on_open" {
		t.Fatalf("locked keys = %#v", locked)
	}
	if values["reader.highlight_color"] != "yellow" || values["reader.page_fit"] != "actual" {
		t.Fatalf("defaults missing: %#v", values)
	}
}

func TestOnlyStudySensitiveKeysAreLockable(t *testing.T) {
	if !Lockable("suggestions.show_on_open") {
		t.Fatal("suggestion visibility must be lockable per cohort")
	}
	if Lockable("colorScheme") || Lockable("nope") {
		t.Fatal("cosmetic or unknown keys must not be lockable")
	}
}

func TestDisplayName(t *testing.T) {
	if got, err := NormalizeDisplayName("  Ada  Lovelace "); err != nil || got != "Ada Lovelace" {
		t.Fatalf("got %q %v", got, err)
	}
	if got, err := NormalizeDisplayName(""); err != nil || got != "" {
		t.Fatalf("empty name should clear: %q %v", got, err)
	}
	for _, bad := range []string{strings.Repeat("a", 81), "bad\x00name", "line\nbreak"} {
		if _, err := NormalizeDisplayName(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestPasswordRules(t *testing.T) {
	email := "selar.qa+x@example.com"
	for _, bad := range []string{"short1", "aaaaaaaaaaaa", "12345678901", "abcdefghijk", email, strings.Repeat("ab1", 30)} {
		if err := CheckPasswordStrength(bad, email); err == nil {
			t.Errorf("password %q should be rejected", bad)
		}
	}
	if err := CheckPasswordStrength("correct horse 42", email); err != nil {
		t.Fatalf("reasonable password rejected: %v", err)
	}
}

func TestSanitizeLegacyDropsUnknownInvalidAndLockedKeys(t *testing.T) {
	got := SanitizeLegacy(map[string]any{
		"colorScheme": "dark", "density": "compact", "reader.zoom": 99.0, "suggestions.show_on_open": false,
	}, []string{"suggestions.show_on_open"})
	if len(got) != 1 || got["colorScheme"] != "dark" {
		t.Fatalf("legacy sanitise = %#v", got)
	}
}
