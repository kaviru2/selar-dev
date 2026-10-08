package notify

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/selar-dev/selar-api/internal/allowlist"
	"github.com/selar-dev/selar-api/internal/calendar"
	"github.com/selar-dev/selar-api/internal/linktoken"
	"github.com/selar-dev/selar-api/internal/quiz"
)

// ---------- fake store ----------

type fakeStore struct {
	mu         sync.Mutex
	recipients map[string]Recipient
	windows    map[string][]calendar.Window
	log        []Claim
	status     map[int64]string
	now        func() time.Time
	created    map[int64]time.Time
}

func newFake(now func() time.Time) *fakeStore {
	return &fakeStore{recipients: map[string]Recipient{}, windows: map[string][]calendar.Window{}, status: map[int64]string{}, created: map[int64]time.Time{}, now: now}
}

func (f *fakeStore) NotificationRecipient(_ context.Context, id string) (Recipient, error) {
	return f.recipients[id], nil
}
func (f *fakeStore) QuizNotificationRecipients(context.Context) ([]Recipient, error) {
	var out []Recipient
	for _, r := range f.recipients {
		if r.QuizEmails {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeStore) CalendarWindows(_ context.Context, id string, _ time.Time) ([]calendar.Window, error) {
	return f.windows[id], nil
}
func (f *fakeStore) ClaimNotification(_ context.Context, c Claim) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.log {
		if x.DedupeKey == c.DedupeKey {
			return 0, false, nil
		}
	}
	f.log = append(f.log, c)
	id := int64(len(f.log))
	f.created[id] = f.now()
	return id, true, nil
}
func (f *fakeStore) FinishNotification(_ context.Context, id int64, status, _ string) error {
	f.mu.Lock()
	f.status[id] = status
	f.mu.Unlock()
	return nil
}
func (f *fakeStore) CountNotificationsSince(_ context.Context, userID string, since time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for i, c := range f.log {
		id := int64(i + 1)
		if (userID == "" || c.UserID == userID) && !f.created[id].Before(since) && f.status[id] != "failed" {
			n++
		}
	}
	return n, nil
}

// recordingTransport stands in for a real transport; it must only be used
// when every switch allows it.
type recordingTransport struct{ sent []Message }

func (r *recordingTransport) Mode() string { return "smtp" }
func (r *recordingTransport) Send(_ context.Context, m Message) error {
	r.sent = append(r.sent, m)
	return nil
}

func tp(s string) *time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return &t
}

const uA, uB = "6f1c1f0e-4b7a-4c39-9a51-0b7d2b1f8c11", "7a2d2a1f-5c8b-4d4a-8b62-1c8e3c2a9d22"

func fixture(now *time.Time) (*fakeStore, Config) {
	st := newFake(func() time.Time { return *now })
	st.recipients[uA] = Recipient{UserID: uA, Email: "a@study.test", QuizEmails: true, SecurityEmails: true}
	st.recipients[uB] = Recipient{UserID: uB, Email: "b@elsewhere.test", QuizEmails: true}
	w := calendar.Window{QuizID: "q1", Kind: quiz.KindFollowUp, Start: tp("2026-10-12T03:30:00Z"), End: tp("2026-10-15T03:30:00Z")}
	st.windows[uA] = []calendar.Window{w}
	st.windows[uB] = []calendar.Window{w}
	return st, Config{
		Allow: allowlist.Parse("@study.test"), DryRun: &DryRun{Quiet: true},
		Signer: linktoken.New("k"), PublicAPIURL: "https://api.test", ConsoleURL: "https://console.test",
		Now: func() time.Time { return *now },
	}
}

// ---------- the safety switches ----------

func TestDefaultConfigNeverSends(t *testing.T) {
	now := *tp("2026-10-12T04:00:00Z")
	st, cfg := fixture(&now)
	real := &recordingTransport{}
	cfg.Real = real // configured transport but NOTIFY_EMAIL_ENABLED unset
	n := New(st, cfg)
	rep, err := n.RunQuizNotices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(real.sent) != 0 {
		t.Fatal("a real transport must not be used without NOTIFY_EMAIL_ENABLED")
	}
	if rep.Transport != ModeDryRun || rep.Outcomes["dry_run"] != 1 {
		t.Fatalf("report = %+v", rep)
	}
	// Only the allowlisted learner was even considered.
	if rep.Recipients != 1 || len(n.DryRunMessages()) != 1 || n.DryRunMessages()[0].To != "a@study.test" {
		t.Fatalf("dry-run = %+v", n.DryRunMessages())
	}
}

func TestLiveRequiresFlagAllowlistAndTransport(t *testing.T) {
	now := *tp("2026-10-12T04:00:00Z")
	for _, c := range []struct {
		name      string
		live      bool
		allow     string
		real      bool
		wantReal  int
		wantDry   int
		wantRecip int
	}{
		{"flag off", false, "*", true, 0, 2, 2},
		{"empty allowlist", true, "", true, 0, 0, 0},
		{"no transport", true, "*", false, 0, 2, 2},
		{"only allowlisted are live", true, "a@study.test", true, 1, 0, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			st, cfg := fixture(&now)
			real := &recordingTransport{}
			cfg.Live, cfg.Allow = c.live, allowlist.Parse(c.allow)
			if c.real {
				cfg.Real = real
			}
			n := New(st, cfg)
			rep, _ := n.RunQuizNotices(context.Background())
			if len(real.sent) != c.wantReal || len(n.DryRunMessages()) != c.wantDry || rep.Recipients != c.wantRecip {
				t.Fatalf("real=%d dry=%d recipients=%d (%+v)", len(real.sent), len(n.DryRunMessages()), rep.Recipients, rep)
			}
		})
	}
}

// ---------- idempotency and limits ----------

func TestNeverDoubleSendsTheSameWindow(t *testing.T) {
	now := *tp("2026-10-12T04:00:00Z")
	st, cfg := fixture(&now)
	n := New(st, cfg)
	for i := 0; i < 3; i++ {
		if _, err := n.RunQuizNotices(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(n.DryRunMessages()); got != 1 {
		t.Fatalf("opening notice sent %d times", got)
	}
	// Closing soon: once, 24h before close, even across many runs.
	now = *tp("2026-10-14T05:00:00Z")
	for i := 0; i < 3; i++ {
		_, _ = n.RunQuizNotices(context.Background())
	}
	msgs := n.DryRunMessages()
	if len(msgs) != 2 || msgs[1].Subject != "SELAR: a quiz closes soon" {
		t.Fatalf("messages = %+v", msgs)
	}
	// A moved window is a new window and may be announced again, once.
	st.windows[uA] = []calendar.Window{{QuizID: "q1", Kind: quiz.KindFollowUp, Start: tp("2026-10-14T04:00:00Z"), End: tp("2026-10-20T00:00:00Z")}}
	_, _ = n.RunQuizNotices(context.Background())
	_, _ = n.RunQuizNotices(context.Background())
	if len(n.DryRunMessages()) != 3 {
		t.Fatalf("moved window: %d messages", len(n.DryRunMessages()))
	}
}

func TestRateLimits(t *testing.T) {
	now := *tp("2026-10-12T04:00:00Z")
	st, cfg := fixture(&now)
	var ws []calendar.Window
	for i := 0; i < 10; i++ {
		ws = append(ws, calendar.Window{QuizID: fmt.Sprintf("q%d", i), Kind: quiz.KindPractice, Start: tp("2026-10-12T03:30:00Z"), End: tp("2026-10-30T00:00:00Z")})
	}
	st.windows[uA] = ws
	cfg.PerUserPerDay = 3
	n := New(st, cfg)
	rep, _ := n.RunQuizNotices(context.Background())
	if len(n.DryRunMessages()) != 3 || rep.Outcomes["rate_limited"] != 7 {
		t.Fatalf("per-user cap: %d %+v", len(n.DryRunMessages()), rep)
	}
	st2, cfg2 := fixture(&now)
	st2.windows[uA] = ws
	cfg2.PerUserPerDay, cfg2.PerRun = 100, 4
	n2 := New(st2, cfg2)
	rep, _ = n2.RunQuizNotices(context.Background())
	if len(n2.DryRunMessages()) != 4 || rep.Stopped == "" {
		t.Fatalf("per-run cap: %d %+v", len(n2.DryRunMessages()), rep)
	}
}

func TestDueQuizNotices(t *testing.T) {
	lead, grace := 24*time.Hour, 6*time.Hour
	w := calendar.Window{Start: tp("2026-10-12T00:00:00Z"), End: tp("2026-10-15T00:00:00Z")}
	for at, want := range map[string]string{
		"2026-10-11T23:00:00Z": "",
		"2026-10-12T01:00:00Z": "quiz_opening",
		"2026-10-12T07:00:00Z": "",
		"2026-10-14T01:00:00Z": "quiz_closing_soon",
		"2026-10-15T00:00:00Z": "",
	} {
		var got []string
		for _, k := range DueQuizNotices(w, *tp(at), lead, grace) {
			got = append(got, string(k))
		}
		if strings.Join(got, ",") != want {
			t.Errorf("%s: %v want %q", at, got, want)
		}
	}
	short := calendar.Window{Start: tp("2026-10-12T00:00:00Z"), End: tp("2026-10-12T08:00:00Z")}
	if got := DueQuizNotices(short, *tp("2026-10-12T01:00:00Z"), lead, grace); len(got) != 1 || got[0] != QuizOpening {
		t.Fatalf("short window: %v", got)
	}
}

// ---------- security notices ----------

func TestSecurityNoticeNeedsOptInAndIsIdempotent(t *testing.T) {
	now := *tp("2026-10-12T04:00:00Z")
	st, cfg := fixture(&now)
	n := New(st, cfg)
	if out := n.Security(context.Background(), uA, "a@study.test", SecurityPassword, "v2"); out != OutcomeDryRun {
		t.Fatalf("out = %s", out)
	}
	if out := n.Security(context.Background(), uA, "a@study.test", SecurityPassword, "v2"); out != OutcomeDuplicate {
		t.Fatalf("repeat = %s", out)
	}
	st.recipients[uA] = Recipient{UserID: uA, Email: "a@study.test"}
	if out := n.Security(context.Background(), uA, "a@study.test", SecurityPassword, "v3"); out != OutcomeNotAllowed {
		t.Fatalf("opted out = %s", out)
	}
	var nilN *Notifier
	if nilN.Security(context.Background(), uA, "x", SecurityEmail, "k") != OutcomeNotAllowed {
		t.Fatal("nil notifier")
	}
}

// ---------- templates ----------

func TestTemplatesAreNeutralAndIdenticalAcrossArms(t *testing.T) {
	colombo, _ := time.LoadLocation("Asia/Colombo")
	d := TemplateData{QuizKind: quiz.KindInitial, Opens: tp("2026-10-12T03:30:00Z"), Closes: tp("2026-10-12T12:00:00Z"), At: *tp("2026-10-12T03:30:00Z"),
		ConsoleURL: "https://console.test", UnsubscribeURL: "https://api.test/notifications/unsubscribe/TOKEN", Location: colombo}
	for _, k := range Kinds {
		subject, body, err := Render(k, d)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ToLower(subject + "\n" + body)
		for _, banned := range []string{"control", "treatment", "hitl", "cohort", "group", "arm ", "condition", "memory", "remember", "retention", "grade", "score", "improve", "!"} {
			if strings.Contains(text, banned) {
				t.Errorf("%s contains %q:\n%s", k, banned, body)
			}
		}
		if !strings.Contains(body, "research prototype") || !strings.Contains(body, d.UnsubscribeURL) {
			t.Errorf("%s: missing prototype note or unsubscribe link", k)
		}
	}
	_, body, _ := Render(QuizOpening, d)
	if !strings.Contains(body, "Mon 12 Oct 2026, 17:30 (Asia/Colombo time)") {
		t.Fatalf("closing time not in Colombo time:\n%s", body)
	}
	if _, _, err := Render(QuizClosingSoon, TemplateData{}); err == nil {
		t.Fatal("closing notice without a close time")
	}
	if _, _, err := Render("nope", d); err == nil {
		t.Fatal("unknown kind")
	}
}

func TestUnsubscribeHeaders(t *testing.T) {
	now := *tp("2026-10-12T04:00:00Z")
	st, cfg := fixture(&now)
	n := New(st, cfg)
	_, _ = n.RunQuizNotices(context.Background())
	m := n.DryRunMessages()[0]
	if !strings.HasPrefix(m.Headers["List-Unsubscribe"], "<https://api.test/notifications/unsubscribe/") || m.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Fatalf("headers = %v", m.Headers)
	}
	tok := strings.TrimSuffix(strings.TrimPrefix(m.Headers["List-Unsubscribe"], "<https://api.test/notifications/unsubscribe/"), ">")
	if id, _, err := linktoken.New("k").Verify(linktoken.Unsubscribe, tok); err != nil || id != uA {
		t.Fatalf("token: %v", err)
	}
	if len(st.log) != 1 || st.log[0].RecipientHash == "" || strings.Contains(st.log[0].RecipientHash, "@") {
		t.Fatal("audit must store a hash, not the address")
	}
}

// ---------- SMTP transport (local fake server, never the internet) ----------

func TestBuildRejectsHeaderInjection(t *testing.T) {
	if _, err := Build("SELAR <s@x.test>", "", Message{To: "a@b.test\r\nBcc: evil@x.test", Subject: "s"}, time.Now()); err == nil {
		t.Fatal("CRLF in To must be rejected")
	}
	raw, err := Build("SELAR <s@x.test>", "", Message{To: "a@b.test", Subject: "Ünïcode", Text: "line1\nline2"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil || msg.Header.Get("Auto-Submitted") != "auto-generated" || !strings.Contains(string(raw), "line1\r\nline2\r\n") {
		t.Fatalf("message: %v\n%s", err, raw)
	}
}

func TestNewSMTPRequiresCompleteConfig(t *testing.T) {
	for _, c := range []SMTPConfig{{}, {Host: "h"}, {Host: "h", From: "x@y.z", Username: "u"}, {Host: "h", From: "bad", Username: "u", Password: "p"}} {
		if _, err := NewSMTP(c); err == nil {
			t.Errorf("%+v accepted", c)
		}
	}
}

func TestSMTPAgainstLocalFakeServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback")
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := func(s string) { fmt.Fprintf(conn, "%s\r\n", s) }
		w("220 fake")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					w("250 queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				w("250-fake")
				w("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "AUTH"):
				w("235 ok")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				w("250 ok")
			case cmd == "DATA":
				inData = true
				w("354 go")
			case cmd == "QUIT":
				w("221 bye")
				got <- data.String()
				return
			default:
				w("250 ok")
			}
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	s, err := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: port, Username: "u", Password: "p", From: "SELAR <selar@x.test>", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// Without STARTTLS the transport refuses to send credentials.
	if err := s.Send(context.Background(), Message{To: "a@x.test", Subject: "s", Text: "t"}); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected STARTTLS refusal, got %v", err)
	}
	select {
	case <-got:
		t.Fatal("no message may be delivered over a connection without TLS")
	default:
	}
}
