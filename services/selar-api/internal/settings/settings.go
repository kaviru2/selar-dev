// Package settings defines the allow-listed, validated per-user settings
// stored in users.preferences, the per-cohort locks that can override them,
// and the account-level validation rules (display name, password strength).
//
// Settings keys are flat dotted strings so other parts of the console can read
// a single key (for example the Reader reads "reader.zoom") without knowing
// the rest of the document.
package settings

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInvalid is wrapped by every validation failure.
var ErrInvalid = errors.New("invalid setting")

type spec struct {
	def      any
	validate func(any) (any, error)
	// lockable marks settings that could change what a study participant is
	// exposed to; administrators may pin them per cohort.
	lockable bool
}

func oneOf(values ...string) func(any) (any, error) {
	return func(v any) (any, error) {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: expected a string", ErrInvalid)
		}
		for _, allowed := range values {
			if s == allowed {
				return s, nil
			}
		}
		return nil, fmt.Errorf("%w: %q is not one of %s", ErrInvalid, s, strings.Join(values, ", "))
	}
}

func boolean(v any) (any, error) {
	b, ok := v.(bool)
	if !ok {
		return nil, fmt.Errorf("%w: expected true or false", ErrInvalid)
	}
	return b, nil
}

// Reader defaults live in one object under the "reader" key, the shape the
// console's Reader reads (lib/reader/position.ts, readerPreferences):
//
//	{"defaultZoom": "fit-width" | "fit-page" | 0.25–4, "rememberPosition": bool,
//	 "showThumbnails": bool, "highlightColor": "yellow"|"green"|"blue"|"pink"|"purple"}
const (
	MinZoom = 0.25
	MaxZoom = 4.0
)

func zoom(v any) (any, error) {
	if s, ok := v.(string); ok {
		if s == "fit-width" || s == "fit-page" {
			return s, nil
		}
		return nil, fmt.Errorf("%w: zoom must be fit-width, fit-page or a number", ErrInvalid)
	}
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("%w: zoom must be fit-width, fit-page or a number", ErrInvalid)
	}
	f = math.Round(f*100) / 100
	if f < MinZoom || f > MaxZoom {
		return nil, fmt.Errorf("%w: zoom must be between %.2f and %.0f", ErrInvalid, MinZoom, MaxZoom)
	}
	return f, nil
}

var readerFields = map[string]spec{
	"defaultZoom":      {def: "fit-width", validate: zoom},
	"rememberPosition": {def: true, validate: boolean},
	"showThumbnails":   {def: false, validate: boolean},
	"highlightColor":   {def: "yellow", validate: oneOf("yellow", "green", "blue", "pink", "purple")},
}

func readerDefaults() map[string]any {
	out := make(map[string]any, len(readerFields))
	for k, f := range readerFields {
		out[k] = f.def
	}
	return out
}

// reader validates a (partial) reader object strictly.
func reader(v any) (any, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, fmt.Errorf("%w: reader must be an object with at least one field", ErrInvalid)
	}
	out := make(map[string]any, len(m))
	for k, raw := range m {
		f, ok := readerFields[k]
		if !ok {
			return nil, fmt.Errorf("%w: unknown reader setting %q", ErrInvalid, k)
		}
		n, err := f.validate(raw)
		if err != nil {
			return nil, fmt.Errorf("reader.%s: %w", k, err)
		}
		out[k] = n
	}
	return out, nil
}

// MergeReader overlays a validated partial reader patch on the stored
// object, dropping stored fields that are unknown or no longer valid.
func MergeReader(stored any, patch map[string]any) map[string]any {
	out := map[string]any{}
	if m, ok := stored.(map[string]any); ok {
		for k, raw := range m {
			if f, ok := readerFields[k]; ok {
				if n, err := f.validate(raw); err == nil {
					out[k] = n
				}
			}
		}
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}

var specs = map[string]spec{
	// Colour scheme, as stored by the console's theme provider
	// (lib/theme.tsx): "system" follows the operating system.
	"colorScheme": {def: "system", validate: oneOf("system", "light", "dark")},
	// Light-mode paper tint. "dark" is accepted for accounts saved before the
	// colour scheme had its own key; the console maps it to colorScheme.
	"theme":                    {def: "paper", validate: oneOf("paper", "warm", "sage", "dark")},
	"reader":                   {def: nil, validate: reader},
	"suggestions.show_on_open": {def: true, validate: boolean, lockable: true},
}

// Keys returns every known setting key, sorted.
func Keys() []string {
	keys := make([]string, 0, len(specs))
	for k := range specs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Defaults returns a fresh copy of the default value of every setting.
func Defaults() map[string]any {
	out := make(map[string]any, len(specs))
	for k, s := range specs {
		out[k] = s.def
	}
	out["reader"] = readerDefaults()
	return out
}

// Lockable reports whether administrators may pin key per cohort.
func Lockable(key string) bool {
	s, ok := specs[key]
	return ok && s.lockable
}

// Validate checks one value for key and returns its normalised form.
func Validate(key string, value any) (any, error) {
	s, ok := specs[key]
	if !ok {
		return nil, fmt.Errorf("%w: unknown setting %q", ErrInvalid, key)
	}
	return s.validate(value)
}

// ValidatePatch validates a partial update. Unknown keys are rejected so
// typos and retired settings cannot silently accumulate.
func ValidatePatch(patch map[string]any) (map[string]any, error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("%w: no settings provided", ErrInvalid)
	}
	out := make(map[string]any, len(patch))
	for k, v := range patch {
		normalised, err := Validate(k, v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = normalised
	}
	return out, nil
}

// SanitizeLegacy filters a whole-document update from the legacy
// preferences route: unknown, invalid and locked keys are dropped instead of
// rejected, because older console builds send every key they hold.
func SanitizeLegacy(patch map[string]any, locked []string) map[string]any {
	out := map[string]any{}
	isLocked := map[string]bool{}
	for _, k := range locked {
		isLocked[k] = true
	}
	for k, v := range patch {
		if isLocked[k] {
			continue
		}
		if normalised, err := Validate(k, v); err == nil {
			out[k] = normalised
		}
	}
	return out
}

// Effective merges defaults, the learner's stored values (ignoring unknown or
// invalid legacy entries) and cohort locks, which always win. It returns the
// values and the sorted list of keys that are locked.
func Effective(stored, locks map[string]any) (map[string]any, []string) {
	values := Defaults()
	for k, v := range stored {
		if k == "reader" {
			values["reader"] = mergeOver(readerDefaults(), MergeReader(v, nil))
			continue
		}
		if normalised, err := Validate(k, v); err == nil {
			values[k] = normalised
		}
	}
	locked := []string{}
	for k, v := range locks {
		if !Lockable(k) {
			continue
		}
		if normalised, err := Validate(k, v); err == nil {
			values[k] = normalised
			locked = append(locked, k)
		}
	}
	sort.Strings(locked)
	return values, locked
}

// MaxDisplayNameRunes bounds the profile display name.
const MaxDisplayNameRunes = 80

// NormalizeDisplayName trims and collapses whitespace. An empty result clears
// the name.
func NormalizeDisplayName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: display name is not valid text", ErrInvalid)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: display name cannot contain control characters", ErrInvalid)
		}
	}
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > MaxDisplayNameRunes {
		return "", fmt.Errorf("%w: display name must be at most %d characters", ErrInvalid, MaxDisplayNameRunes)
	}
	return name, nil
}

// Password rules. bcrypt only uses the first 72 bytes, so longer passwords
// are rejected rather than silently truncated.
const (
	MinPasswordLength   = 10
	MaxPasswordBytes    = 72
	minDistinctPassword = 4
)

// CheckPasswordStrength enforces the change-password rules: 10–72 bytes,
// at least one letter and one non-letter, not a single repeated character,
// and not the account email.
func CheckPasswordStrength(password, email string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrInvalid, MinPasswordLength)
	}
	if len(password) > MaxPasswordBytes {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrInvalid, MaxPasswordBytes)
	}
	var letter, other bool
	distinct := map[rune]bool{}
	for _, r := range password {
		distinct[unicode.ToLower(r)] = true
		if unicode.IsLetter(r) {
			letter = true
		} else {
			other = true
		}
	}
	if !letter || !other {
		return fmt.Errorf("%w: password must mix letters with numbers, spaces or symbols", ErrInvalid)
	}
	if len(distinct) < minDistinctPassword {
		return fmt.Errorf("%w: password is too repetitive", ErrInvalid)
	}
	if email != "" && strings.Contains(strings.ToLower(password), strings.ToLower(email)) {
		return fmt.Errorf("%w: password must not contain your email address", ErrInvalid)
	}
	return nil
}

func mergeOver(base, top map[string]any) map[string]any {
	for k, v := range top {
		base[k] = v
	}
	return base
}
