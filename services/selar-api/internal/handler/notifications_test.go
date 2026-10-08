package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/allowlist"
	"github.com/selar-dev/selar-api/internal/linktoken"
	"github.com/selar-dev/selar-api/internal/notify"
	"github.com/selar-dev/selar-api/internal/quiz"
)

func TestNotificationRunRequiresSecret(t *testing.T) {
	h := New(nil)
	h.SetNotifier(nil, "")
	r := chi.NewRouter()
	h.MountNotificationPublic(r)
	for _, secret := range []string{"", "wrong"} {
		req := httptest.NewRequest(http.MethodPost, "/internal/notifications/run", nil)
		req.Header.Set("X-Selar-Worker-Secret", secret)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("secret %q: %d", secret, rec.Code)
		}
	}
	h.SetNotifier(nil, "s3cret")
	req := httptest.NewRequest(http.MethodPost, "/internal/notifications/run", nil)
	req.Header.Set("X-Selar-Worker-Secret", "wrong")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret: %d", rec.Code)
	}
}

func TestUnsubscribeRejectsBadTokensAndGetIsSafe(t *testing.T) {
	h := New(nil)
	h.SetCalendarConfig(CalendarConfig{Signer: linktoken.New("k")})
	h.SetNotifier(notify.New(nil, notify.Config{}), "")
	r := chi.NewRouter()
	h.MountNotificationPublic(r)
	calTok, _ := linktoken.New("k").Issue(linktoken.Calendar, "6f1c1f0e-4b7a-4c39-9a51-0b7d2b1f8c11", 0)
	for _, tok := range []string{"nope", calTok} {
		for _, m := range []string{http.MethodGet, http.MethodPost} {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(m, "/notifications/unsubscribe/"+tok, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s %s: %d", m, tok, rec.Code)
			}
		}
	}
	// A valid token's GET only shows a form (store is nil: any write would panic).
	tok, _ := linktoken.New("k").Issue(linktoken.Unsubscribe, "6f1c1f0e-4b7a-4c39-9a51-0b7d2b1f8c11", 0)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/notifications/unsubscribe/"+tok, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `<form method="post">`) {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
}

func TestIntegrationEmailNoticesDryRunEndToEnd(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	open, close := now.Add(-time.Hour), now.Add(72*time.Hour)
	quizID, err := f.st.CreateQuiz(ctx, f.id, quiz.Draft{
		Title: "SECRET-TITLE notify", Kind: quiz.KindFollowUp,
		Settings:  quiz.Settings{OpenAt: &open, CloseAt: &close, Feedback: quiz.FeedbackNever, Audience: quiz.Audience{Type: quiz.AudienceGroup, Value: "notify-test-" + f.id}},
		Questions: []quiz.Question{{Type: quiz.TypeTrueFalse, Prompt: "p", Points: 1, Options: []quiz.Option{{Text: "True", Correct: true}, {Text: "False"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM quizzes WHERE id = $1`, quizID)
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM quiz_group_members WHERE label = $1`, "notify-test-"+f.id)
	})
	if err := f.st.SetQuizStatus(ctx, quizID, quiz.StatusPublished); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO quiz_group_members(label,user_id) VALUES ($1,$2)`, "notify-test-"+f.id, f.id); err != nil {
		t.Fatal(err)
	}

	// Configuration identical to production defaults except for the allowlist.
	dry := &notify.DryRun{Quiet: true}
	signer := linktoken.New("test-only-secret")
	n := notify.New(f.st, notify.Config{Allow: allowlist.Parse(f.email), DryRun: dry, Signer: signer,
		PublicAPIURL: "https://api.test", ConsoleURL: "https://console.test"})
	f.h.SetNotifier(n, "run-secret")
	f.h.SetCalendarConfig(CalendarConfig{Signer: signer})

	// Off by default.
	rec := sendJSON(t, f.h.GetNotificationSettings, http.MethodGet, "/", "", f.id)
	if !strings.Contains(rec.Body.String(), `"available":true,"quiz_emails":false,"security_emails":false,"live":false`) {
		t.Fatalf("defaults: %s", rec.Body.String())
	}
	if rec := sendJSON(t, f.h.UpdateNotificationSettings, http.MethodPut, "/", `{"quiz_emails":true,"security_emails":true}`, f.id); rec.Code != 200 {
		t.Fatalf("opt in: %d %s", rec.Code, rec.Body.String())
	}

	pub := chi.NewRouter()
	f.h.MountNotificationPublic(pub)
	run := func() notify.RunReport {
		req := httptest.NewRequest(http.MethodPost, "/internal/notifications/run", nil)
		req.Header.Set("X-Selar-Worker-Secret", "run-secret")
		rec := httptest.NewRecorder()
		pub.ServeHTTP(rec, req)
		var rep notify.RunReport
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &rep) != nil {
			t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
		}
		return rep
	}
	rep := run()
	if rep.Transport != "dry-run" || rep.Outcomes["dry_run"] < 1 {
		t.Fatalf("first run: %+v", rep)
	}
	run()
	run()
	var rows int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM notification_log WHERE user_id=$1 AND quiz_id=$2`, f.id, quizID).Scan(&rows)
	if rows != 1 {
		t.Fatalf("expected exactly one audit row for the window, got %d", rows)
	}
	var status, transport, hash string
	_ = f.pool.QueryRow(ctx, `SELECT status, transport, recipient_hash FROM notification_log WHERE user_id=$1 AND quiz_id=$2`, f.id, quizID).Scan(&status, &transport, &hash)
	if status != "dry_run" || transport != "dry-run" || strings.Contains(hash, "@") {
		t.Fatalf("audit row: %s %s %s", status, transport, hash)
	}
	var mine []notify.Message
	for _, m := range dry.Sent() {
		if m.To == f.email {
			mine = append(mine, m)
		}
	}
	if len(mine) != 1 || strings.Contains(mine[0].Text, "SECRET-TITLE") || strings.Contains(strings.ToLower(mine[0].Text), "control") {
		t.Fatalf("dry-run messages: %+v", mine)
	}

	// Security notice on password change: dry-run, one per session version.
	auth := f.router()
	body := `{"current_password":"` + fixturePassword + `","new_password":"another synthetic 9"}`
	tok, _ := f.auth.GenerateToken(f.id)
	req := httptest.NewRequest(http.MethodPost, "/api/users/me/password", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	prec := httptest.NewRecorder()
	auth.ServeHTTP(prec, req)
	if prec.Code != 200 {
		t.Fatalf("password change: %d %s", prec.Code, prec.Body.String())
	}
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM notification_log WHERE user_id=$1 AND kind='security_password_changed' AND status='dry_run'`, f.id).Scan(&rows)
	if rows != 1 {
		t.Fatalf("security notice rows = %d", rows)
	}

	// Unsubscribe link from the message turns everything off; reuse is harmless.
	link := strings.TrimSuffix(strings.TrimPrefix(mine[0].Headers["List-Unsubscribe"], "<https://api.test"), ">")
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		pub.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, link, nil))
		if rec.Code != 200 {
			t.Fatalf("unsubscribe: %d %s", rec.Code, rec.Body.String())
		}
	}
	rec = sendJSON(t, f.h.GetNotificationSettings, http.MethodGet, "/", "", f.id)
	if !strings.Contains(rec.Body.String(), `"quiz_emails":false,"security_emails":false`) {
		t.Fatalf("after unsubscribe: %s", rec.Body.String())
	}

	// Not allowlisted: cannot opt in.
	f.h.SetNotifier(notify.New(f.st, notify.Config{}), "run-secret")
	if rec := sendJSON(t, f.h.UpdateNotificationSettings, http.MethodPut, "/", `{"quiz_emails":true}`, f.id); rec.Code != http.StatusForbidden {
		t.Fatalf("opt-in without allowlist: %d", rec.Code)
	}
	// Opting out is always allowed.
	if rec := sendJSON(t, f.h.UpdateNotificationSettings, http.MethodPut, "/", `{"quiz_emails":false}`, f.id); rec.Code != 200 {
		t.Fatalf("opt-out: %d", rec.Code)
	}
}
