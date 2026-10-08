package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig configures the SMTP transport. It works with any provider that
// offers SMTP submission, including a dedicated Google Workspace or Gmail
// project mailbox using an app password. No OAuth token is involved.
type SMTPConfig struct {
	Host     string
	Port     int // 587 (STARTTLS) or 465 (implicit TLS)
	Username string
	Password string
	From     string // "SELAR research prototype <selar@example.org>"
	ReplyTo  string
	// Timeout bounds the whole SMTP conversation.
	Timeout time.Duration
	// insecureLocal allows plain connections to localhost (tests only).
	insecureLocal bool
}

// SMTP sends mail over SMTP submission.
type SMTP struct{ cfg SMTPConfig }

// NewSMTP validates cfg. It returns ErrNotConfigured when anything required
// is missing, so a half-configured environment stays in dry-run.
func NewSMTP(cfg SMTPConfig) (*SMTP, error) {
	if cfg.Host == "" || cfg.From == "" || cfg.Username == "" || cfg.Password == "" {
		return nil, ErrNotConfigured
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("%w: invalid from address", ErrNotConfigured)
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &SMTP{cfg: cfg}, nil
}

func (s *SMTP) Mode() string { return "smtp" }

// Build renders the RFC 5322 message (exported for tests).
func Build(from, replyTo string, m Message, now time.Time) ([]byte, error) {
	for _, v := range []string{m.To, m.Subject, from, replyTo} {
		if err := headerSafe(v); err != nil {
			return nil, err
		}
	}
	h := map[string]string{
		"From":                      from,
		"To":                        m.To,
		"Subject":                   mime.QEncoding.Encode("utf-8", m.Subject),
		"Date":                      now.UTC().Format(time.RFC1123Z),
		"MIME-Version":              "1.0",
		"Content-Type":              "text/plain; charset=utf-8",
		"Content-Transfer-Encoding": "8bit",
		"Auto-Submitted":            "auto-generated",
	}
	if replyTo != "" {
		h["Reply-To"] = replyTo
	}
	for k, v := range m.Headers {
		if err := headerSafe(k + v); err != nil {
			return nil, err
		}
		h[k] = v
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + ": " + h[k] + "\r\n")
	}
	b.WriteString("\r\n")
	body := strings.ReplaceAll(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\n", "\r\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\r\n") {
		b.WriteString("\r\n")
	}
	return []byte(b.String()), nil
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("invalid recipient")
	}
	from, _ := mail.ParseAddress(s.cfg.From)
	raw, err := Build(s.cfg.From, s.cfg.ReplyTo, m, time.Now())
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	deadline := time.Now().Add(s.cfg.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	dialer := &net.Dialer{Deadline: deadline}
	var conn net.Conn
	tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
	if s.cfg.Port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp connect: %w", err)
	}
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp hello: %w", err)
	}
	defer c.Close()
	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		} else if !s.cfg.insecureLocal {
			return fmt.Errorf("smtp server does not offer STARTTLS; refusing to send credentials in clear text")
		}
	}
	if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp end data: %w", err)
	}
	return c.Quit()
}
