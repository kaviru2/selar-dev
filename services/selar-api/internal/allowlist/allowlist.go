// Package allowlist implements the email allowlists that gate participant-
// facing features which are switched off by default (calendar feeds, email
// notices). A feature is live for an account only when its master flag is
// true AND the account's address matches the allowlist, so turning a feature
// on for real participants is always an explicit, two-step decision.
package allowlist

import "strings"

// List matches email addresses. Entries are exact addresses
// ("ana@example.com"), whole domains ("@example.com") or "*" (everyone).
// The zero value matches nothing.
type List struct {
	all     bool
	emails  map[string]struct{}
	domains map[string]struct{}
}

// Parse reads a comma-, space- or semicolon-separated list. Matching is
// case-insensitive. Malformed entries are ignored.
func Parse(raw string) List {
	l := List{emails: map[string]struct{}{}, domains: map[string]struct{}{}}
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' })
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		switch {
		case f == "*":
			l.all = true
		case strings.HasPrefix(f, "@") && len(f) > 1 && !strings.Contains(f[1:], "@"):
			l.domains[f[1:]] = struct{}{}
		case strings.Count(f, "@") == 1 && !strings.HasPrefix(f, "@") && !strings.HasSuffix(f, "@"):
			l.emails[f] = struct{}{}
		}
	}
	return l
}

// Allows reports whether email matches the list.
func (l List) Allows(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return false
	}
	if l.all {
		return true
	}
	if _, ok := l.emails[email]; ok {
		return true
	}
	_, ok := l.domains[email[at+1:]]
	return ok
}

// Everyone reports whether the list is "*".
func (l List) Everyone() bool { return l.all }

// Size is the number of explicit entries (addresses and domains).
func (l List) Size() int { return len(l.emails) + len(l.domains) }

// Flag parses a boolean feature flag. Only an explicit true value turns a
// feature on; anything else (unset, typos) leaves it off.
func Flag(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Gate combines a master flag and an allowlist.
type Gate struct {
	Enabled bool
	List    List
}

// FromEnv builds a gate from a flag value and an allowlist value.
func FromEnv(flag, list string) Gate { return Gate{Enabled: Flag(flag), List: Parse(list)} }

// Allows reports whether the feature is live for email.
func (g Gate) Allows(email string) bool { return g.Enabled && g.List.Allows(email) }

// Describe is a log-safe summary (no addresses).
func (g Gate) Describe() string {
	switch {
	case !g.Enabled:
		return "off"
	case g.List.Everyone():
		return "on for everyone"
	case g.List.Size() == 0:
		return "on but allowlist empty (effectively off)"
	default:
		return "on for allowlisted accounts only"
	}
}
