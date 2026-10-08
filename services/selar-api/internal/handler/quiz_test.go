package handler_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/selar-dev/selar-api/internal/handler"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/quiz"
	"github.com/selar-dev/selar-api/internal/store"
)

type fixedAdmin bool

func (f fixedAdmin) IsAdmin(context.Context, string) (bool, error) { return bool(f), nil }

func adminRouter(h *handler.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "u1")))
		})
	})
	h.MountQuizRoutes(r)
	return r
}

func TestQuizAdminRoutesRejectNonAdmins(t *testing.T) {
	h := handler.New(nil)
	h.SetAdminChecker(fixedAdmin(false))
	router := adminRouter(h)
	for _, path := range []string{"/admin/quizzes", "/admin/quizzes/x", "/admin/quizzes/x/results.csv", "/admin/quiz-attempts/x", "/admin/quiz-groups"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusForbidden {
			t.Errorf("GET %s as non-admin: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/quizzes/import", strings.NewReader("title: x")))
	if w.Code != http.StatusForbidden {
		t.Errorf("import as non-admin: %d", w.Code)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/access", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"admin":false`) {
		t.Errorf("access probe: %d %s", w.Code, w.Body.String())
	}
}

func TestQuizAdminDeniedWhenNoCheckerConfigured(t *testing.T) {
	router := adminRouter(handler.New(nil))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/quizzes", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("no checker must fail closed, got %d", w.Code)
	}
}

func TestQuizImportDryRunValidatesWithoutStore(t *testing.T) {
	h := handler.New(nil)
	h.SetAdminChecker(fixedAdmin(true))
	router := adminRouter(h)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/quizzes/import?filename=q.md&dry_run=1",
		strings.NewReader("# DEMO\n## true_false\nSky is blue\n- [x] True\n- [ ] False\n")))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"title":"DEMO"`) {
		t.Fatalf("dry run: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/admin/quizzes/import?filename=q.yaml&dry_run=1", strings.NewReader("title: x")))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "at least one question") {
		t.Fatalf("invalid import: %d %s", w.Code, w.Body.String())
	}
}

func TestEmailAllowlistAdminChecker(t *testing.T) {
	lookup := func(_ context.Context, id string) (string, error) {
		return map[string]string{"a": "Admin@Example.com", "b": "b@example.com"}[id], nil
	}
	c := handler.NewEmailAllowlist(" admin@example.com , ", lookup)
	if ok, _ := c.IsAdmin(context.Background(), "a"); !ok {
		t.Fatal("allowlisted email (case-insensitive) rejected")
	}
	if ok, _ := c.IsAdmin(context.Background(), "b"); ok {
		t.Fatal("non-listed email accepted")
	}
	if ok, _ := handler.NewEmailAllowlist("", lookup).IsAdmin(context.Background(), "a"); ok {
		t.Fatal("empty allowlist must deny everyone")
	}
}

// The downloadable templates shipped by the console must import cleanly.
func TestPublishedQuizTemplatesParse(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "selar-console", "public", "quiz-templates")
	for _, name := range []string{"quiz-template.yaml", "quiz-template.md", "quiz-template.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		d, err := quiz.Parse(name, data)
		if err != nil {
			t.Fatalf("%s does not import: %v", name, err)
		}
		if !strings.Contains(d.Title, "DEMO") || len(d.Questions) < 6 {
			t.Fatalf("%s should be a DEMO quiz covering every question type: %q, %d questions", name, d.Title, len(d.Questions))
		}
	}
}

func TestQuizCSVNeutralisesFormulasAndAggregatesAttempts(t *testing.T) {
	two, zero := 2.0, 0.0
	yes, no := true, false
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)
	rows := []store.ExportRow{
		{AttemptID: "a1", UserID: "u1", Cohort: "control", AttemptNumber: 1, QuizVersion: 1, StartedAt: start, SubmittedAt: &end, QuestionIndex: 0, Prompt: "p", Points: 2, Selected: []string{"o2"}, SelectedText: []string{"B"}, AutoScore: &two, FinalScore: &two, IsCorrect: &yes},
		{AttemptID: "a1", UserID: "u1", Cohort: "control", AttemptNumber: 1, QuizVersion: 1, StartedAt: start, SubmittedAt: &end, QuestionIndex: 1, Prompt: "p", Points: 3, AnswerText: "=HYPERLINK(\"x\")", AutoScore: &zero, FinalScore: &zero, IsCorrect: &no},
		{AttemptID: "a1", UserID: "u1", Cohort: "control", AttemptNumber: 1, QuizVersion: 1, StartedAt: start, SubmittedAt: &end, QuestionIndex: 2, Prompt: "p", Points: 5, AnswerText: "recall", NeedsReview: true},
	}
	var answers bytes.Buffer
	if err := handler.WriteQuizCSV(&answers, "answer", rows); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(answers.String(), ",=HYPERLINK") || !strings.Contains(answers.String(), "'=HYPERLINK") {
		t.Fatalf("formula not neutralised:\n%s", answers.String())
	}
	if strings.Contains(answers.String(), "@") {
		t.Fatalf("export must not contain emails:\n%s", answers.String())
	}
	var attempts bytes.Buffer
	_ = handler.WriteQuizCSV(&attempts, "attempt", rows)
	lines := strings.Split(strings.TrimSpace(attempts.String()), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[1], ",90.0,3,3,10,,1,") {
		t.Fatalf("attempt row (pending review must leave score blank):\n%s", attempts.String())
	}
}
