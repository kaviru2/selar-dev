package quiz

import "testing"

func q(kind QuestionType, points float64, opts ...Option) Question {
	return Question{ID: "x", Type: kind, Points: points, Options: opts}
}

func TestGradeSingleChoice(t *testing.T) {
	question := q(TypeSingleChoice, 2, Option{ID: "a"}, Option{ID: "b", Correct: true})
	g := Grade(question, Answer{Selected: []string{"b"}})
	if !g.Auto || g.Score == nil || *g.Score != 2 || g.Correct == nil || !*g.Correct {
		t.Fatalf("correct single: %+v", g)
	}
	g = Grade(question, Answer{Selected: []string{"a"}})
	if *g.Score != 0 || *g.Correct {
		t.Fatalf("wrong single: %+v", g)
	}
	g = Grade(question, Answer{Selected: []string{"a", "b"}})
	if *g.Score != 0 {
		t.Fatalf("two picks on single choice must not score: %+v", g)
	}
	g = Grade(question, Answer{})
	if *g.Score != 0 || *g.Correct {
		t.Fatalf("blank single: %+v", g)
	}
}

func TestGradeMultipleChoiceIsAllOrNothing(t *testing.T) {
	question := q(TypeMultipleChoice, 3, Option{ID: "a", Correct: true}, Option{ID: "b", Correct: true}, Option{ID: "c"})
	if g := Grade(question, Answer{Selected: []string{"b", "a"}}); *g.Score != 3 {
		t.Fatalf("exact set: %+v", g)
	}
	if g := Grade(question, Answer{Selected: []string{"a"}}); *g.Score != 0 {
		t.Fatalf("partial set must score 0: %+v", g)
	}
	if g := Grade(question, Answer{Selected: []string{"a", "b", "c"}}); *g.Score != 0 {
		t.Fatalf("superset must score 0: %+v", g)
	}
	if g := Grade(question, Answer{Selected: []string{"a", "a", "b"}}); *g.Score != 3 {
		t.Fatalf("duplicates are deduplicated: %+v", g)
	}
}

func TestGradeTrueFalse(t *testing.T) {
	question := q(TypeTrueFalse, 1, Option{ID: "true", Correct: true}, Option{ID: "false"})
	if g := Grade(question, Answer{Selected: []string{"true"}}); *g.Score != 1 {
		t.Fatalf("true: %+v", g)
	}
	if g := Grade(question, Answer{Selected: []string{"false"}}); *g.Score != 0 {
		t.Fatalf("false: %+v", g)
	}
}

func TestGradeShortAnswerNormalisesAndOtherwiseNeedsReview(t *testing.T) {
	question := Question{Type: TypeShortAnswer, Points: 1, AcceptedAnswers: []string{"Spaced repetition"}}
	if g := Grade(question, Answer{Text: "  spaced   REPETITION. "}); !g.Auto || *g.Score != 1 {
		t.Fatalf("normalised match: %+v", g)
	}
	g := Grade(question, Answer{Text: "repetition with gaps"})
	if g.Auto || g.Score != nil || !g.NeedsReview {
		t.Fatalf("non-matching short answer should go to manual review, got %+v", g)
	}
	g = Grade(question, Answer{Text: ""})
	if !g.Auto || *g.Score != 0 {
		t.Fatalf("blank short answer scores zero automatically: %+v", g)
	}
	noKey := Question{Type: TypeShortAnswer, Points: 1}
	if g := Grade(noKey, Answer{Text: "anything"}); !g.NeedsReview {
		t.Fatalf("short answer without key needs review: %+v", g)
	}
}

func TestGradeRecallAlwaysNeedsManualReview(t *testing.T) {
	for _, kind := range []QuestionType{TypeFreeRecall, TypeCuedRecall} {
		g := Grade(Question{Type: kind, Points: 5, AcceptedAnswers: []string{"x"}}, Answer{Text: "x"})
		if g.Auto || g.Score != nil || !g.NeedsReview {
			t.Fatalf("%s must be manually scored: %+v", kind, g)
		}
		g = Grade(Question{Type: kind, Points: 5}, Answer{Text: "   "})
		if !g.Auto || *g.Score != 0 {
			t.Fatalf("%s blank should score 0 automatically: %+v", kind, g)
		}
	}
}

func TestSummariseAttemptScores(t *testing.T) {
	two, zero := 2.0, 0.0
	s := Summarise([]Graded{{Points: 2, Score: &two}, {Points: 3, Score: &zero}, {Points: 5, NeedsReview: true}})
	if s.MaxPoints != 10 || s.AutoPoints != 2 || s.PendingReview != 1 || s.Score != nil {
		t.Fatalf("pending summary: %+v", s)
	}
	five := 4.5
	s = Summarise([]Graded{{Points: 2, Score: &two}, {Points: 5, Score: &five}})
	if s.Score == nil || *s.Score != 6.5 || s.PendingReview != 0 || s.Percent == nil || *s.Percent != 6.5/7*100 {
		t.Fatalf("final summary: %+v", s)
	}
}
