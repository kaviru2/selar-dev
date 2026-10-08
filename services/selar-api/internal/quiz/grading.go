package quiz

import (
	"sort"
	"strings"
	"unicode"
)

type QuestionType string

const (
	TypeSingleChoice   QuestionType = "single_choice"
	TypeMultipleChoice QuestionType = "multiple_choice"
	TypeTrueFalse      QuestionType = "true_false"
	TypeShortAnswer    QuestionType = "short_answer"
	TypeFreeRecall     QuestionType = "free_recall"
	TypeCuedRecall     QuestionType = "cued_recall"
)

// QuestionTypes lists every supported type in display order.
var QuestionTypes = []QuestionType{TypeSingleChoice, TypeMultipleChoice, TypeTrueFalse, TypeShortAnswer, TypeFreeRecall, TypeCuedRecall}

// IsChoice reports whether answers are option selections.
func (t QuestionType) IsChoice() bool {
	return t == TypeSingleChoice || t == TypeMultipleChoice || t == TypeTrueFalse
}

// Valid reports whether t is a supported type.
func (t QuestionType) Valid() bool {
	for _, v := range QuestionTypes {
		if v == t {
			return true
		}
	}
	return false
}

type Option struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Correct bool   `json:"correct,omitempty"`
}

// Question is the full (admin-side) question including its key.
type Question struct {
	ID              string       `json:"id"`
	Position        int          `json:"position"`
	Type            QuestionType `json:"type"`
	Prompt          string       `json:"prompt"`
	Cue             string       `json:"cue,omitempty"`
	Options         []Option     `json:"options,omitempty"`
	AcceptedAnswers []string     `json:"accepted_answers,omitempty"`
	Points          float64      `json:"points"`
	Rubric          string       `json:"rubric,omitempty"`
	Explanation     string       `json:"explanation,omitempty"`
	SourceDocument  string       `json:"source_document,omitempty"`
	SourceURL       string       `json:"source_url,omitempty"`
}

// Answer is what a learner submitted for one question.
type Answer struct {
	Selected []string `json:"selected,omitempty"`
	Text     string   `json:"text,omitempty"`
}

// Graded is the outcome of grading one answer. Score is nil while the answer
// waits for manual review.
type Graded struct {
	Points      float64
	Score       *float64
	Correct     *bool
	Auto        bool
	NeedsReview bool
}

func scored(points float64, correct bool) Graded {
	s := 0.0
	if correct {
		s = points
	}
	return Graded{Points: points, Score: &s, Correct: &correct, Auto: true}
}

// Normalise lowercases, trims, collapses whitespace and strips trailing
// punctuation so short answers match independent of formatting.
func Normalise(s string) string {
	s = strings.Join(strings.Fields(strings.ToLower(s)), " ")
	return strings.TrimRightFunc(s, func(r rune) bool { return unicode.IsPunct(r) })
}

func uniqueSorted(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Grade auto-grades objective questions. Short answers that do not match an
// accepted answer, and all non-blank recall answers, are left for a human.
func Grade(q Question, a Answer) Graded {
	if q.Type.IsChoice() {
		var key []string
		for _, o := range q.Options {
			if o.Correct {
				key = append(key, o.ID)
			}
		}
		picked := uniqueSorted(a.Selected)
		key = uniqueSorted(key)
		if q.Type != TypeMultipleChoice && len(picked) > 1 {
			return scored(q.Points, false)
		}
		return scored(q.Points, len(key) > 0 && strings.Join(picked, "\x00") == strings.Join(key, "\x00"))
	}
	text := Normalise(a.Text)
	if text == "" {
		return scored(q.Points, false)
	}
	if q.Type == TypeShortAnswer {
		for _, accepted := range q.AcceptedAnswers {
			if Normalise(accepted) == text {
				return scored(q.Points, true)
			}
		}
	}
	return Graded{Points: q.Points, NeedsReview: true}
}

type Summary struct {
	MaxPoints     float64  `json:"max_points"`
	AutoPoints    float64  `json:"auto_points"`
	PendingReview int      `json:"pending_review"`
	Score         *float64 `json:"score,omitempty"`
	Percent       *float64 `json:"percent,omitempty"`
}

// Summarise totals graded answers. The final Score is only set when no
// answer is still waiting for review.
func Summarise(items []Graded) Summary {
	var s Summary
	for _, g := range items {
		s.MaxPoints += g.Points
		if g.Score == nil {
			s.PendingReview++
			continue
		}
		s.AutoPoints += *g.Score
	}
	if s.PendingReview == 0 {
		total := s.AutoPoints
		s.Score = &total
		if s.MaxPoints > 0 {
			pct := total / s.MaxPoints * 100
			s.Percent = &pct
		}
	}
	return s
}
