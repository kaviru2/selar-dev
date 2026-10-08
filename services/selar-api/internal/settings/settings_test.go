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
	got, err := ValidatePatch(decode(t, `{"colorScheme":"dark","theme":"warm","reader":{"defaultZoom":1.25,"highlightColor":"green"},"suggestions.show_on_open":false}`))
	if err != nil {
		t.Fatalf("valid patch rejected: %v", err)
	}
	if got["colorScheme"] != "dark" || got["theme"] != "warm" ||
		readerField(got, "defaultZoom") != 1.25 || readerField(got, "highlightColor") != "green" || got["suggestions.show_on_open"] != false {
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
		`{"reader.zoom":1}`,
		`{"reader":"fit-width"}`,
		`{"reader":{}}`,
		`{"reader":{"defaultZoom":5}}`,
		`{"reader":{"defaultZoom":0.1}}`,
		`{"reader":{"defaultZoom":"1"}}`,
		`{"reader":{"defaultZoom":"stretch"}}`,
		`{"reader":{"highlightColor":"#ff0000"}}`,
		`{"reader":{"rememberPosition":"yes"}}`,
		`{"reader":{"unknown":true}}`,
		`{"suggestions.show_on_open":"no"}`,
		`{"cohort":"control"}`,
	} {
		if _, err := ValidatePatch(decode(t, raw)); err == nil {
			t.Errorf("patch %s should be rejected", raw)
		}
	}
}

func readerField(m map[string]any, k string) any {
	r, _ := m["reader"].(map[string]any)
	return r[k]
}

func TestValidatePatchRoundsZoomAndAcceptsFitModes(t *testing.T) {
	got, err := ValidatePatch(decode(t, `{"reader":{"defaultZoom":1.2345}}`))
	if err != nil || readerField(got, "defaultZoom") != 1.23 {
		t.Fatalf("zoom = %#v, %v", got, err)
	}
	for _, mode := range []string{"fit-width", "fit-page"} {
		got, err := ValidatePatch(map[string]any{"reader": map[string]any{"defaultZoom": mode, "rememberPosition": false, "showThumbnails": true, "highlightColor": "purple"}})
		if err != nil || readerField(got, "defaultZoom") != mode || readerField(got, "highlightColor") != "purple" {
			t.Fatalf("%s: %#v %v", mode, got, err)
		}
	}
}

func TestMergeReaderKeepsEarlierFieldsAndDropsInvalidStored(t *testing.T) {
	stored := map[string]any{"defaultZoom": 9.0, "highlightColor": "blue", "junk": 1}
	merged := MergeReader(stored, map[string]any{"showThumbnails": true})
	if merged["highlightColor"] != "blue" || merged["showThumbnails"] != true {
		t.Fatalf("merged = %#v", merged)
	}
	if _, ok := merged["defaultZoom"]; ok {
		t.Fatal("invalid stored zoom must be dropped")
	}
	if _, ok := merged["junk"]; ok {
		t.Fatal("unknown stored field must be dropped")
	}
}

func TestEffectiveAppliesDefaultsDropsUnknownAndLocksWin(t *testing.T) {
	stored := map[string]any{"colorScheme": "dark", "density": "compact", "reader": map[string]any{"defaultZoom": 9.0, "highlightColor": "pink"}, "suggestions.show_on_open": false}
	locks := map[string]any{"suggestions.show_on_open": true}
	values, locked := Effective(stored, locks)
	if values["colorScheme"] != "dark" || values["theme"] != "paper" {
		t.Fatalf("stored colour scheme lost: %#v", values)
	}
	if _, ok := values["density"]; ok {
		t.Fatal("unknown legacy key must not be exposed")
	}
	if readerField(values, "defaultZoom") != "fit-width" || readerField(values, "highlightColor") != "pink" {
		t.Fatalf("reader must merge per field over defaults, got %#v", values["reader"])
	}
	if values["suggestions.show_on_open"] != true {
		t.Fatal("cohort lock must override the learner's stored value")
	}
	if len(locked) != 1 || locked[0] != "suggestions.show_on_open" {
		t.Fatalf("locked keys = %#v", locked)
	}
	if readerField(values, "rememberPosition") != true || readerField(values, "showThumbnails") != false {
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
		"colorScheme": "dark", "density": "compact", "reader": map[string]any{"defaultZoom": 99.0, "highlightColor": "green"}, "suggestions.show_on_open": false,
	}, []string{"suggestions.show_on_open"})
	if len(got) != 1 || got["colorScheme"] != "dark" {
		t.Fatalf("legacy sanitise = %#v", got)
	}
}
