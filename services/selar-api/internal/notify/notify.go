// Package notify sends SELAR's optional study notices by email: a quiz window
// opening, a quiz closing soon, and account security notices.
//
// Safety model (issue #114):
//
//   - Every message goes through a Transport. The default is the dry-run
//     transport, which only logs a redacted line and records an audit row; it
//     never contacts a mail server.
//   - A real transport (SMTP) is used for a recipient only when
//     NOTIFY_EMAIL_ENABLED=true AND the address matches NOTIFY_EMAIL_ALLOWLIST
//     AND an SMTP transport is fully configured. Anything else falls back to
//     dry-run for that message.
//   - Participants must opt in per notice family in Settings (default off).
//   - Each (notice, learner, quiz, window) is claimed once in
//     notification_log before sending, so a window is never notified twice,
//     even by concurrent runs. A failed send is not retried.
//   - Per-learner and global daily caps and a per-run cap bound volume.
//   - Templates are identical for every study arm and never mention cohorts,
//     groups, quiz titles, scores or claims about memory or learning.
package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Message is one plain-text email.
type Message struct {
	To      string
	Subject string
	Text    string
	// Headers are extra headers (List-Unsubscribe and friends).
	Headers map[string]string
}

// Transport delivers messages.
type Transport interface {
	// Mode is recorded in the audit log: "dry-run" or "smtp".
	Mode() string
	Send(ctx context.Context, m Message) error
}

// ModeDryRun is the mode of the logging transport.
const ModeDryRun = "dry-run"

// DryRun records messages and logs a redacted line. It never sends.
type DryRun struct {
	mu   sync.Mutex
	sent []Message
	// Quiet disables log output (tests).
	Quiet bool
}

func (d *DryRun) Mode() string { return ModeDryRun }

func (d *DryRun) Send(_ context.Context, m Message) error {
	d.mu.Lock()
	d.sent = append(d.sent, m)
	if len(d.sent) > 100 {
		d.sent = d.sent[len(d.sent)-100:]
	}
	d.mu.Unlock()
	if !d.Quiet {
		log.Printf("notify dry-run: to=%s subject=%q bytes=%d (not sent)", RecipientHash(m.To)[:12], m.Subject, len(m.Text))
	}
	return nil
}

// Sent returns a copy of the recorded messages (most recent 100).
func (d *DryRun) Sent() []Message {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Message(nil), d.sent...)
}

// RecipientHash identifies a recipient in logs and the audit table without
// storing the address itself.
func RecipientHash(email string) string {
	sum := sha256.Sum256([]byte("selar-notify-recipient\x00" + strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

// ErrNotConfigured is returned by a transport missing required settings.
var ErrNotConfigured = errors.New("email transport not configured")

func headerSafe(s string) error {
	if strings.ContainsAny(s, "\r\n") {
		return fmt.Errorf("header value contains a line break")
	}
	return nil
}

// Clock is overridable in tests.
type Clock func() time.Time
