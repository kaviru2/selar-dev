// quiz.go — HTTP handlers for the quiz system (issue #86).
//
// Learner routes live under /api/quizzes and /api/quiz-attempts; admin routes
// under /api/admin/... and pass through requireAdmin. Until the shared admin
// role lands, admin status comes from an AdminChecker (by default an
// ADMIN_EMAILS allowlist). With no checker configured every admin request is
// refused.
package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/analytics"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/quiz"
	"github.com/selar-dev/selar-api/internal/store"
)

// AdminChecker decides whether a user may use admin routes.
type AdminChecker interface {
	IsAdmin(ctx context.Context, userID string) (bool, error)
}

// SetAdminChecker configures admin authorisation for quiz admin routes.
func (h *Handler) SetAdminChecker(c AdminChecker) { h.admins = c }

type emailAllowlist struct {
	emails map[string]bool
	lookup func(context.Context, string) (string, error)
}

// NewEmailAllowlist builds an AdminChecker from a comma-separated email list.
func NewEmailAllowlist(csvEmails string, lookup func(context.Context, string) (string, error)) AdminChecker {
	set := map[string]bool{}
	for _, e := range strings.Split(csvEmails, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			set[e] = true
		}
	}
	return emailAllowlist{emails: set, lookup: lookup}
}

func (a emailAllowlist) IsAdmin(ctx context.Context, userID string) (bool, error) {
	if len(a.emails) == 0 || userID == "" {
		return false, nil
	}
	email, err := a.lookup(ctx, userID)
	if err != nil {
		return false, err
	}
	return a.emails[strings.ToLower(strings.TrimSpace(email))], nil
}

func (h *Handler) isAdmin(r *http.Request) bool {
	if h.admins == nil {
		return false
	}
	ok, err := h.admins.IsAdmin(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		log.Printf("admin check failed: %v", err)
		return false
	}
	return ok
}

func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.isAdmin(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MountQuizRoutes registers learner and admin quiz routes on an authenticated router.
func (h *Handler) MountQuizRoutes(r chi.Router) {
	r.Get("/admin/access", h.AdminAccess)
	r.Get("/quizzes", h.ListLearnerQuizzes)
	r.Post("/quizzes/{id}/attempts", h.StartQuizAttempt)
	r.Get("/quiz-attempts/{id}", h.GetQuizAttempt)
	r.Put("/quiz-attempts/{id}/answers", h.SaveQuizAnswer)
	r.Post("/quiz-attempts/{id}/advance", h.AdvanceQuizAttempt)
	r.Post("/quiz-attempts/{id}/submit", h.SubmitQuizAttempt)
	r.Get("/quiz-attempts/{id}/result", h.GetQuizResult)

	r.Group(func(r chi.Router) {
		r.Use(h.requireAdmin)
		r.Get("/admin/quizzes", h.AdminListQuizzes)
		r.Post("/admin/quizzes", h.AdminCreateQuiz)
		r.Post("/admin/quizzes/import", h.AdminImportQuiz)
		r.Get("/admin/quizzes/{id}", h.AdminGetQuiz)
		r.Put("/admin/quizzes/{id}", h.AdminUpdateQuiz)
		r.Delete("/admin/quizzes/{id}", h.AdminDeleteQuiz)
		r.Post("/admin/quizzes/{id}/status", h.AdminSetQuizStatus)
		r.Post("/admin/quizzes/{id}/duplicate", h.AdminDuplicateQuiz)
		r.Get("/admin/quizzes/{id}/export", h.AdminExportQuizFile)
		r.Get("/admin/quizzes/{id}/attempts", h.AdminListAttempts)
		r.Get("/admin/quizzes/{id}/stats", h.AdminQuestionStats)
		r.Get("/admin/quizzes/{id}/results.csv", h.AdminResultsCSV)
		r.Get("/admin/quiz-attempts/{id}", h.AdminGetAttempt)
		r.Put("/admin/quiz-answers/{id}/grade", h.AdminGradeAnswer)
		r.Get("/admin/quiz-groups", h.AdminListGroups)
		r.Put("/admin/quiz-groups/{label}", h.AdminSetGroup)
	})
}

func quizError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrQuizNotFound), errors.Is(err, store.ErrAttemptNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrQuizUnavailable), errors.Is(err, store.ErrAttemptSubmitted),
		errors.Is(err, store.ErrAttemptNotSubmitted), errors.Is(err, store.ErrNoGoingBack), errors.Is(err, store.ErrQuizLocked):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, store.ErrInvalidAnswer), errors.Is(err, store.ErrInvalidGrade):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		log.Printf("quiz request failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "quiz request failed"})
	}
}

func uid(r *http.Request) string { return middleware.GetUserID(r.Context()) }

// ---------- learner ----------

func (h *Handler) AdminAccess(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"admin": h.isAdmin(r)})
}

func (h *Handler) ListLearnerQuizzes(w http.ResponseWriter, r *http.Request) {
	cards, err := h.store.ListQuizzesForLearner(r.Context(), uid(r), time.Now())
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cards)
}

func (h *Handler) StartQuizAttempt(w http.ResponseWriter, r *http.Request) {
	view, err := h.store.StartQuizAttempt(r.Context(), uid(r), chi.URLParam(r, "id"), time.Now())
	if err != nil {
		quizError(w, err)
		return
	}
	h.trackQuizAttempt(r.Context(), uid(r), view.AttemptID, analytics.QuizStarted)
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) GetQuizAttempt(w http.ResponseWriter, r *http.Request) {
	view, err := h.store.GetAttemptView(r.Context(), uid(r), chi.URLParam(r, "id"), time.Now())
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) SaveQuizAnswer(w http.ResponseWriter, r *http.Request) {
	var in store.SaveAnswerInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.store.SaveQuizAnswer(r.Context(), uid(r), chi.URLParam(r, "id"), in, time.Now()); err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved_at": time.Now().UTC()})
}

func (h *Handler) AdvanceQuizAttempt(w http.ResponseWriter, r *http.Request) {
	view, err := h.store.AdvanceQuizAttempt(r.Context(), uid(r), chi.URLParam(r, "id"), time.Now())
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) SubmitQuizAttempt(w http.ResponseWriter, r *http.Request) {
	// Submission is idempotent; only the first submit is recorded as an event.
	alreadySubmitted := h.quizAttemptSubmitted(r.Context(), uid(r), chi.URLParam(r, "id"))
	if err := h.store.SubmitQuizAttempt(r.Context(), uid(r), chi.URLParam(r, "id"), time.Now()); err != nil {
		quizError(w, err)
		return
	}
	if !alreadySubmitted {
		h.trackQuizAttempt(r.Context(), uid(r), chi.URLParam(r, "id"), analytics.QuizSubmitted)
	}
	h.GetQuizResult(w, r)
}

func (h *Handler) GetQuizResult(w http.ResponseWriter, r *http.Request) {
	res, err := h.store.QuizResult(r.Context(), uid(r), chi.URLParam(r, "id"), time.Now())
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ---------- admin ----------

func (h *Handler) AdminListQuizzes(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListQuizzesAdmin(r.Context())
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func decodeDraft(w http.ResponseWriter, r *http.Request) (quiz.Draft, bool) {
	var d quiz.Draft
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := dec.Decode(&d); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return d, false
	}
	if err := d.Normalise(); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return d, false
	}
	return d, true
}

func (h *Handler) AdminCreateQuiz(w http.ResponseWriter, r *http.Request) {
	d, ok := decodeDraft(w, r)
	if !ok {
		return
	}
	id, err := h.store.CreateQuiz(r.Context(), uid(r), d)
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// AdminImportQuiz creates a draft quiz from a YAML, JSON or Markdown file in
// the request body. ?filename= picks the format; ?dry_run=1 only validates
// and returns the parsed quiz for preview.
func (h *Handler) AdminImportQuiz(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "quiz files are limited to 1 MB"})
		return
	}
	name := r.URL.Query().Get("filename")
	if name == "" {
		name = "quiz.yaml"
		if strings.Contains(r.Header.Get("Content-Type"), "markdown") {
			name = "quiz.md"
		}
	}
	d, err := quiz.Parse(name, body)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if r.URL.Query().Get("dry_run") != "" {
		writeJSON(w, http.StatusOK, map[string]any{"draft": d})
		return
	}
	id, err := h.store.CreateQuiz(r.Context(), uid(r), d)
	if err != nil {
		quizError(w, err)
		return
	}
	if r.URL.Query().Get("publish") == "1" {
		if err := h.store.SetQuizStatus(r.Context(), id, quiz.StatusPublished); err != nil {
			quizError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "draft": d})
}

func (h *Handler) AdminGetQuiz(w http.ResponseWriter, r *http.Request) {
	rec, err := h.store.GetQuiz(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (h *Handler) AdminUpdateQuiz(w http.ResponseWriter, r *http.Request) {
	d, ok := decodeDraft(w, r)
	if !ok {
		return
	}
	if err := h.store.UpdateQuiz(r.Context(), chi.URLParam(r, "id"), d); err != nil {
		quizError(w, err)
		return
	}
	h.AdminGetQuiz(w, r)
}

func (h *Handler) AdminDeleteQuiz(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteQuiz(r.Context(), chi.URLParam(r, "id")); err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) AdminSetQuizStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status quiz.Status `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	switch body.Status {
	case quiz.StatusDraft, quiz.StatusPublished, quiz.StatusClosed:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be draft, published or closed"})
		return
	}
	if err := h.store.SetQuizStatus(r.Context(), chi.URLParam(r, "id"), body.Status); err != nil {
		quizError(w, err)
		return
	}
	h.AdminGetQuiz(w, r)
}

func (h *Handler) AdminDuplicateQuiz(w http.ResponseWriter, r *http.Request) {
	id, err := h.store.DuplicateQuiz(r.Context(), uid(r), chi.URLParam(r, "id"))
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

var unsafeFileChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func fileSlug(title string) string {
	s := strings.Trim(unsafeFileChars.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "quiz"
	}
	return s
}

func (h *Handler) AdminExportQuizFile(w http.ResponseWriter, r *http.Request) {
	rec, err := h.store.GetQuiz(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		quizError(w, err)
		return
	}
	out, err := quiz.ExportYAML(rec.Draft)
	if err != nil {
		quizError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.yaml"`, fileSlug(rec.Title)))
	_, _ = w.Write(out)
}

func (h *Handler) AdminListAttempts(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListQuizAttempts(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) AdminQuestionStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.QuizQuestionStats(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) AdminGetAttempt(w http.ResponseWriter, r *http.Request) {
	d, err := h.store.GetAttemptForAdmin(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) AdminGradeAnswer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Score *float64 `json:"score"`
		Note  string   `json:"note"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.store.GradeQuizAnswer(r.Context(), uid(r), chi.URLParam(r, "id"), body.Score, body.Note); err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "graded"})
}

func (h *Handler) AdminListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.store.ListQuizGroups(r.Context())
	if err != nil {
		quizError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *Handler) AdminSetGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Emails []string `json:"emails"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	missing, err := h.store.SetQuizGroupMembers(r.Context(), chi.URLParam(r, "label"), body.Emails)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unknown_emails": missing})
}

// ---------- CSV export ----------

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func fmtFloat(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}

func fmtBool(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

// csvSafe neutralises spreadsheet formula injection in learner-written text.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// AdminResultsCSV exports ?level=answer (default, one row per attempt ×
// question) or ?level=attempt (one row per attempt). Rows use pseudonymous
// user ids, never emails.
func (h *Handler) AdminResultsCSV(w http.ResponseWriter, r *http.Request) {
	quizID := chi.URLParam(r, "id")
	rec, err := h.store.GetQuiz(r.Context(), quizID)
	if err != nil {
		quizError(w, err)
		return
	}
	rows, err := h.store.QuizExportRows(r.Context(), quizID)
	if err != nil {
		quizError(w, err)
		return
	}
	level := r.URL.Query().Get("level")
	if level != "attempt" {
		level = "answer"
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, fileSlug(rec.Title), level))
	_ = WriteQuizCSV(w, level, rows)
}

// WriteQuizCSV renders export rows at "answer" or "attempt" level.
func WriteQuizCSV(out io.Writer, level string, rows []store.ExportRow) error {
	cw := csv.NewWriter(out)
	if level == "attempt" {
		_ = cw.Write([]string{"attempt_id", "user_id", "cohort", "attempt_number", "quiz_version", "started_at", "submitted_at",
			"submit_reason", "duration_seconds", "questions", "answered", "max_points", "score", "pending_review", "percent"})
		type agg struct {
			r                    store.ExportRow
			n, answered, pending int
			max, score           float64
		}
		var order []string
		by := map[string]*agg{}
		for _, row := range rows {
			a := by[row.AttemptID]
			if a == nil {
				a = &agg{r: row}
				by[row.AttemptID] = a
				order = append(order, row.AttemptID)
			}
			a.n++
			a.max += row.Points
			if row.AnswerText != "" || len(row.Selected) > 0 {
				a.answered++
			}
			if row.NeedsReview {
				a.pending++
			} else if row.FinalScore != nil {
				a.score += *row.FinalScore
			}
		}
		for _, id := range order {
			a := by[id]
			duration, score, pct := "", "", ""
			if a.r.SubmittedAt != nil {
				duration = strconv.FormatFloat(a.r.SubmittedAt.Sub(a.r.StartedAt).Seconds(), 'f', 1, 64)
			}
			if a.pending == 0 {
				score = strconv.FormatFloat(a.score, 'f', -1, 64)
				if a.max > 0 {
					pct = strconv.FormatFloat(a.score/a.max*100, 'f', 1, 64)
				}
			}
			reason := ""
			if a.r.SubmitReason != nil {
				reason = *a.r.SubmitReason
			}
			_ = cw.Write([]string{a.r.AttemptID, a.r.UserID, a.r.Cohort, strconv.Itoa(a.r.AttemptNumber), strconv.Itoa(a.r.QuizVersion),
				fmtTime(&a.r.StartedAt), fmtTime(a.r.SubmittedAt), reason, duration, strconv.Itoa(a.n), strconv.Itoa(a.answered),
				strconv.FormatFloat(a.max, 'f', -1, 64), score, strconv.Itoa(a.pending), pct})
		}
	} else {
		_ = cw.Write([]string{"attempt_id", "user_id", "cohort", "attempt_number", "quiz_version", "submitted_at", "question_order",
			"question_id", "question_type", "prompt", "selected_option_ids", "selected_option_text", "answer_text", "points",
			"auto_score", "manual_score", "final_score", "is_correct", "pending_review", "grader_note", "time_on_question_ms",
			"first_seen_at", "answered_at", "revisions"})
		for _, row := range rows {
			_ = cw.Write([]string{row.AttemptID, row.UserID, row.Cohort, strconv.Itoa(row.AttemptNumber), strconv.Itoa(row.QuizVersion),
				fmtTime(row.SubmittedAt), strconv.Itoa(row.QuestionIndex + 1), row.QuestionID, row.QuestionType, csvSafe(row.Prompt),
				strings.Join(row.Selected, "|"), csvSafe(strings.Join(row.SelectedText, "|")), csvSafe(row.AnswerText),
				strconv.FormatFloat(row.Points, 'f', -1, 64), fmtFloat(row.AutoScore), fmtFloat(row.ManualScore), fmtFloat(row.FinalScore),
				fmtBool(row.IsCorrect), strconv.FormatBool(row.NeedsReview), csvSafe(row.GraderNote), strconv.FormatInt(row.TimeOnQuestionMs, 10),
				fmtTime(&row.FirstSeenAt), fmtTime(row.AnsweredAt), strconv.Itoa(row.Revisions)})
		}
	}
	cw.Flush()
	return cw.Error()
}
