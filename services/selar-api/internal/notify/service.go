package notify

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/selar-dev/selar-api/internal/allowlist"
	"github.com/selar-dev/selar-api/internal/calendar"
	"github.com/selar-dev/selar-api/internal/linktoken"
)

// Recipient is an opted-in learner.
type Recipient struct {
	UserID             string
	Email              string
	QuizEmails         bool
	SecurityEmails     bool
	UnsubscribeVersion int
}

// Claim is the audit row written before a send.
type Claim struct {
	UserID        string
	Kind          Kind
	DedupeKey     string
	QuizID        string
	WindowStart   *time.Time
	WindowEnd     *time.Time
	RecipientHash string
	Transport     string
	Subject       string
}

// Store is the persistence the notifier needs (implemented by store.Store).
type Store interface {
	NotificationRecipient(ctx context.Context, userID string) (Recipient, error)
	QuizNotificationRecipients(ctx context.Context) ([]Recipient, error)
	CalendarWindows(ctx context.Context, userID string, now time.Time) ([]calendar.Window, error)
	// ClaimNotification inserts a pending audit row; claimed is false when a
	// row with the same dedupe key already exists.
	ClaimNotification(ctx context.Context, c Claim) (id int64, claimed bool, err error)
	FinishNotification(ctx context.Context, id int64, status, errText string) error
	// CountNotificationsSince counts delivered (sent or dry-run) notices;
	// userID "" counts everyone.
	CountNotificationsSince(ctx context.Context, userID string, since time.Time) (int, error)
}

// Config holds the switches. The zero value sends nothing anywhere.
type Config struct {
	// Live is NOTIFY_EMAIL_ENABLED. Without it every message is dry-run.
	Live bool
	// Allow is NOTIFY_EMAIL_ALLOWLIST: who may receive notices at all
	// (dry-run or real) and who sees the opt-in in Settings.
	Allow allowlist.List
	// Real is the transport used when Live and Allow both permit; nil keeps
	// everything in dry-run.
	Real Transport
	// DryRun records what would have been sent.
	DryRun *DryRun

	Signer       *linktoken.Signer
	PublicAPIURL string
	ConsoleURL   string
	Location     *time.Location // time zone used in email text

	PerUserPerDay int           // default 3
	PerDay        int           // default 200
	PerRun        int           // default 50
	ClosingLead   time.Duration // default 24h
	OpeningGrace  time.Duration // default 6h: never announce a window opened longer ago
	Now           Clock
}

// Notifier sends study notices.
type Notifier struct {
	cfg   Config
	store Store
}

// New applies defaults.
func New(st Store, cfg Config) *Notifier {
	if cfg.DryRun == nil {
		cfg.DryRun = &DryRun{}
	}
	if cfg.PerUserPerDay <= 0 {
		cfg.PerUserPerDay = 3
	}
	if cfg.PerDay <= 0 {
		cfg.PerDay = 200
	}
	if cfg.PerRun <= 0 {
		cfg.PerRun = 50
	}
	if cfg.ClosingLead <= 0 {
		cfg.ClosingLead = 24 * time.Hour
	}
	if cfg.OpeningGrace <= 0 {
		cfg.OpeningGrace = 6 * time.Hour
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Location == nil {
		cfg.Location = time.UTC
	}
	return &Notifier{cfg: cfg, store: st}
}

// Available reports whether email may be offered to this address at all.
func (n *Notifier) Available(email string) bool { return n != nil && n.cfg.Allow.Allows(email) }

// LiveFor reports whether a message to email would really be sent.
func (n *Notifier) LiveFor(email string) bool {
	return n != nil && n.cfg.Live && n.cfg.Real != nil && n.cfg.Allow.Allows(email)
}

// Mode is a log-safe summary of the effective mode.
func (n *Notifier) Mode() string {
	switch {
	case n == nil || n.cfg.Allow.Size() == 0 && !n.cfg.Allow.Everyone():
		return "off (allowlist empty)"
	case !n.cfg.Live || n.cfg.Real == nil:
		return "dry-run (nothing is sent)"
	case n.cfg.Allow.Everyone():
		return "live for every opted-in account"
	default:
		return "live for allowlisted opted-in accounts"
	}
}

// TransportMode is "dry-run" unless real sending is possible for someone.
func (n *Notifier) TransportMode() string {
	if n != nil && n.cfg.Live && n.cfg.Real != nil {
		return n.cfg.Real.Mode()
	}
	return ModeDryRun
}

// DryRunMessages exposes the dry-run record (tests and admin status).
func (n *Notifier) DryRunMessages() []Message { return n.cfg.DryRun.Sent() }

func (n *Notifier) transportFor(email string) Transport {
	if n.LiveFor(email) {
		return n.cfg.Real
	}
	return n.cfg.DryRun
}

// UnsubscribeURL is the signed one-click unsubscribe link.
func (n *Notifier) UnsubscribeURL(r Recipient) string {
	if n.cfg.Signer == nil || n.cfg.PublicAPIURL == "" {
		return ""
	}
	tok, err := n.cfg.Signer.Issue(linktoken.Unsubscribe, r.UserID, r.UnsubscribeVersion)
	if err != nil {
		return ""
	}
	return strings.TrimRight(n.cfg.PublicAPIURL, "/") + "/notifications/unsubscribe/" + tok
}

// Outcome of one attempted notice.
type Outcome string

const (
	OutcomeSent        Outcome = "sent"
	OutcomeDryRun      Outcome = "dry_run"
	OutcomeDuplicate   Outcome = "duplicate"
	OutcomeRateLimited Outcome = "rate_limited"
	OutcomeNotAllowed  Outcome = "not_allowed"
	OutcomeFailed      Outcome = "failed"
)

var errRateLimited = errors.New("rate limited")

func (n *Notifier) deliver(ctx context.Context, r Recipient, kind Kind, key string, quizID string, w *calendar.Window, data TemplateData) (Outcome, error) {
	if !n.cfg.Allow.Allows(r.Email) {
		return OutcomeNotAllowed, nil
	}
	now := n.cfg.Now()
	if c, err := n.store.CountNotificationsSince(ctx, r.UserID, now.Add(-24*time.Hour)); err != nil {
		return OutcomeFailed, err
	} else if c >= n.cfg.PerUserPerDay {
		return OutcomeRateLimited, nil
	}
	if c, err := n.store.CountNotificationsSince(ctx, "", now.Add(-24*time.Hour)); err != nil {
		return OutcomeFailed, err
	} else if c >= n.cfg.PerDay {
		return OutcomeRateLimited, errRateLimited
	}
	data.ConsoleURL, data.Location, data.UnsubscribeURL = n.cfg.ConsoleURL, n.cfg.Location, n.UnsubscribeURL(r)
	subject, body, err := Render(kind, data)
	if err != nil {
		return OutcomeFailed, err
	}
	t := n.transportFor(r.Email)
	claim := Claim{UserID: r.UserID, Kind: kind, DedupeKey: key, QuizID: quizID, RecipientHash: RecipientHash(r.Email), Transport: t.Mode(), Subject: subject}
	if w != nil {
		claim.WindowStart, claim.WindowEnd = w.Start, w.End
	}
	id, claimed, err := n.store.ClaimNotification(ctx, claim)
	if err != nil {
		return OutcomeFailed, err
	}
	if !claimed {
		return OutcomeDuplicate, nil
	}
	msg := Message{To: r.Email, Subject: subject, Text: body, Headers: map[string]string{}}
	if data.UnsubscribeURL != "" {
		msg.Headers["List-Unsubscribe"] = "<" + data.UnsubscribeURL + ">"
		msg.Headers["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
	}
	sendErr := t.Send(ctx, msg)
	status := string(OutcomeSent)
	if t.Mode() == ModeDryRun {
		status = string(OutcomeDryRun)
	}
	errText := ""
	if sendErr != nil {
		status, errText = string(OutcomeFailed), truncate(sendErr.Error(), 300)
	}
	if err := n.store.FinishNotification(ctx, id, status, errText); err != nil {
		log.Printf("notify: audit update failed: %v", err)
	}
	if sendErr != nil {
		return OutcomeFailed, sendErr
	}
	return Outcome(status), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// RunReport summarises one scheduled run. It never contains addresses.
type RunReport struct {
	Mode       string         `json:"mode"`
	Transport  string         `json:"transport"`
	Recipients int            `json:"recipients"`
	Outcomes   map[string]int `json:"outcomes"`
	Stopped    string         `json:"stopped,omitempty"`
}

func windowKey(kind Kind, userID string, w calendar.Window) string {
	unix := func(t *time.Time) string {
		if t == nil {
			return "-"
		}
		return strconv.FormatInt(t.Unix(), 10)
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s", kind, userID, w.QuizID, unix(w.Start), unix(w.End))
}

// DueQuizNotices decides which quiz notices are due for one window at now.
// Pure, so the scheduling rules are unit-tested without a database.
func DueQuizNotices(w calendar.Window, now time.Time, lead, grace time.Duration) []Kind {
	var due []Kind
	open := w.Start == nil || !now.Before(*w.Start)
	if !open || (w.End != nil && !now.Before(*w.End)) {
		return nil
	}
	if w.Start != nil && now.Sub(*w.Start) <= grace {
		due = append(due, QuizOpening)
	}
	if w.End != nil && w.End.Sub(now) <= lead {
		// A window shorter than the lead time only gets the opening notice.
		if w.Start == nil || w.End.Sub(*w.Start) > lead {
			due = append(due, QuizClosingSoon)
		}
	}
	return due
}

// RunQuizNotices sends every due quiz notice to opted-in learners once.
func (n *Notifier) RunQuizNotices(ctx context.Context) (RunReport, error) {
	rep := RunReport{Mode: n.Mode(), Transport: n.TransportMode(), Outcomes: map[string]int{}}
	recipients, err := n.store.QuizNotificationRecipients(ctx)
	if err != nil {
		return rep, err
	}
	now := n.cfg.Now()
	attempted := 0
	for _, r := range recipients {
		if !r.QuizEmails || !n.cfg.Allow.Allows(r.Email) {
			continue
		}
		rep.Recipients++
		windows, err := n.store.CalendarWindows(ctx, r.UserID, now)
		if err != nil {
			return rep, err
		}
		for _, w := range windows {
			for _, kind := range DueQuizNotices(w, now, n.cfg.ClosingLead, n.cfg.OpeningGrace) {
				if attempted >= n.cfg.PerRun {
					rep.Stopped = "per-run limit reached"
					return rep, nil
				}
				win := w
				out, err := n.deliver(ctx, r, kind, windowKey(kind, r.UserID, w), w.QuizID, &win,
					TemplateData{QuizKind: w.Kind, Opens: w.Start, Closes: w.End})
				rep.Outcomes[string(out)]++
				if out != OutcomeDuplicate {
					attempted++
				}
				if errors.Is(err, errRateLimited) {
					rep.Stopped = "daily limit reached"
					return rep, nil
				}
				if err != nil {
					log.Printf("notify: %s failed: %v", kind, err)
				}
			}
		}
	}
	return rep, nil
}

// Security sends an account security notice to email (for an email change,
// the previous address) if the learner opted in. eventKey makes the notice
// idempotent (for example the new session version). It never returns an
// error to the caller's request path; failures are logged and audited.
func (n *Notifier) Security(ctx context.Context, userID, email string, kind Kind, eventKey string) Outcome {
	if n == nil || n.store == nil {
		return OutcomeNotAllowed
	}
	r, err := n.store.NotificationRecipient(ctx, userID)
	if err != nil || !r.SecurityEmails {
		return OutcomeNotAllowed
	}
	r.Email = email
	out, err := n.deliver(ctx, r, kind, fmt.Sprintf("%s:%s:%s", kind, userID, eventKey), "", nil, TemplateData{At: n.cfg.Now()})
	if err != nil {
		log.Printf("notify: security notice failed: %v", err)
	}
	return out
}
