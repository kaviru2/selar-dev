package quiz

import (
	"reflect"
	"strings"
	"testing"
)

func TestShuffleIsDeterministicPerSeedAndAPermutation(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	one := ShuffledIDs(ids, "attempt-1")
	again := ShuffledIDs(ids, "attempt-1")
	if !reflect.DeepEqual(one, again) {
		t.Fatalf("same seed gave different orders: %v %v", one, again)
	}
	if reflect.DeepEqual(one, ids) && reflect.DeepEqual(ShuffledIDs(ids, "attempt-2"), ids) {
		t.Fatal("shuffle never changes order")
	}
	seen := map[string]bool{}
	for _, id := range one {
		seen[id] = true
	}
	if len(seen) != len(ids) || len(one) != len(ids) {
		t.Fatalf("not a permutation: %v", one)
	}
	if &one[0] == &ids[0] {
		t.Fatal("input slice was reused")
	}
}

const yamlQuiz = `
title: "DEMO: Practice quiz"
description: Invented demo content.
kind: follow_up
feedback: after_close
max_attempts: 2
time_limit_minutes: 15
open_at: "2026-10-10T09:00:00+05:30"
audience: "cohort:control"
after: {anchor: quiz_submitted, quiz_id: 11111111-1111-1111-1111-111111111111, days: 7, window_days: 3}
shuffle_questions: true
shuffle_options: true
no_going_back: true
questions:
  - type: single_choice
    prompt: Which colour is the invented Zorb fruit?
    options: ["Red", "*Blue", "Green"]
    points: 2
    explanation: The demo text says blue.
  - type: multiple_choice
    prompt: Pick the two invented planets.
    options:
      - {text: Quilla, correct: true}
      - {text: Mars}
      - {text: Brennos, correct: true}
  - type: true_false
    prompt: Zorbs grow underwater.
    answer: false
  - type: short_answer
    prompt: Name the Zorb's home valley.
    answers: [Vell, "Vell Valley"]
  - type: cued_recall
    prompt: Complete the phrase.
    cue: "Zorbs ripen when ..."
    rubric: Full credit for mentioning the second moon.
    source: {document: "DEMO reading", url: "https://example.com/demo"}
  - type: free_recall
    prompt: Write everything you remember about Zorbs.
    points: 5
`

func TestParseYAMLQuiz(t *testing.T) {
	d, err := Parse("quiz.yaml", []byte(yamlQuiz))
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "DEMO: Practice quiz" || d.Kind != KindFollowUp || d.Settings.Feedback != FeedbackAfterClose ||
		d.Settings.MaxAttempts != 2 || d.Settings.TimeLimitSeconds != 900 || !d.Settings.NoGoingBack ||
		!d.Settings.ShuffleOptions || !d.Settings.ShuffleQuestions {
		t.Fatalf("settings: %+v", d)
	}
	if d.Settings.OpenAt == nil || d.Settings.OpenAt.UTC().Format("15:04") != "03:30" {
		t.Fatalf("open_at with offset: %v", d.Settings.OpenAt)
	}
	if d.Settings.Audience != (Audience{Type: AudienceCohort, Value: "control"}) {
		t.Fatalf("audience: %+v", d.Settings.Audience)
	}
	if a := d.Settings.After; a == nil || a.Anchor != AnchorQuiz || a.Days != 7 || a.WindowDays != 3 {
		t.Fatalf("after: %+v", a)
	}
	if len(d.Questions) != 6 {
		t.Fatalf("questions: %d", len(d.Questions))
	}
	sc := d.Questions[0]
	if sc.Points != 2 || len(sc.Options) != 3 || !sc.Options[1].Correct || sc.Options[1].Text != "Blue" || sc.Options[0].ID == sc.Options[1].ID {
		t.Fatalf("single choice: %+v", sc)
	}
	if mc := d.Questions[1]; mc.Points != 1 || !mc.Options[0].Correct || mc.Options[1].Correct || !mc.Options[2].Correct {
		t.Fatalf("multiple choice: %+v", mc)
	}
	tf := d.Questions[2]
	if len(tf.Options) != 2 || tf.Options[0].ID != "true" || tf.Options[0].Correct || !tf.Options[1].Correct {
		t.Fatalf("true/false: %+v", tf)
	}
	if sa := d.Questions[3]; !reflect.DeepEqual(sa.AcceptedAnswers, []string{"Vell", "Vell Valley"}) {
		t.Fatalf("short answer: %+v", sa)
	}
	cr := d.Questions[4]
	if cr.Cue != "Zorbs ripen when ..." || cr.Rubric == "" || cr.SourceDocument != "DEMO reading" || cr.SourceURL != "https://example.com/demo" {
		t.Fatalf("cued recall: %+v", cr)
	}
	for i, q := range d.Questions {
		if q.Position != i {
			t.Fatalf("position %d = %d", i, q.Position)
		}
	}
}

func TestParseJSONQuiz(t *testing.T) {
	d, err := Parse("quiz.json", []byte(`{"title":"DEMO json","questions":[{"type":"true_false","prompt":"Sky is green","answer":false}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != KindPractice || d.Settings.Feedback != FeedbackAfterSubmit || d.Settings.MaxAttempts != 1 || d.Settings.Audience.Type != AudienceAll {
		t.Fatalf("defaults: %+v", d)
	}
	if !d.Questions[0].Options[1].Correct {
		t.Fatalf("tf: %+v", d.Questions[0])
	}
}

// Feedback after an initial or follow-up test is itself a learning event, so
// study-style kinds default to never revealing answers unless the admin opts in.
func TestStudyKindsDefaultToNoFeedback(t *testing.T) {
	for _, kind := range []string{"initial", "follow_up"} {
		d, err := Parse("q.yaml", []byte("title: x\nkind: "+kind+"\nquestions: [{type: free_recall, prompt: x}]"))
		if err != nil || d.Settings.Feedback != FeedbackNever {
			t.Fatalf("%s default feedback = %q (%v)", kind, d.Settings.Feedback, err)
		}
	}
}

const markdownQuiz = `---
kind: initial
feedback: never
no_going_back: true
---
# DEMO: Markdown quiz

Invented content for QA only.
Second description line.

## single_choice (2 points)
Which invented river feeds Vell?
- [ ] The Orn
- [x] The Sable

Explanation: The demo reading names the Sable.

## multiple_choice
Pick both invented moons.
- [x] Ith
- [x] Ombra
- [ ] Luna

## true_false
Zorbs are blue.
- [x] True
- [ ] False

## short_answer (1 point)
Name the valley.
Answer: Vell
Answer: Vell Valley

## cued_recall
Recall the ripening rule.
Cue: Zorbs ripen when ...
Rubric: Credit any mention of the second moon.
Source: DEMO reading

## free_recall (5 points)
Write everything you remember.
It can span lines.
`

func TestParseMarkdownQuiz(t *testing.T) {
	d, err := Parse("quiz.md", []byte(markdownQuiz))
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "DEMO: Markdown quiz" || d.Description != "Invented content for QA only.\nSecond description line." || d.Kind != KindInitial || d.Settings.Feedback != FeedbackNever || !d.Settings.NoGoingBack {
		t.Fatalf("header: %+v", d)
	}
	if len(d.Questions) != 6 {
		t.Fatalf("question count %d", len(d.Questions))
	}
	q0 := d.Questions[0]
	if q0.Type != TypeSingleChoice || q0.Points != 2 || q0.Prompt != "Which invented river feeds Vell?" || !q0.Options[1].Correct || q0.Explanation == "" {
		t.Fatalf("q0: %+v", q0)
	}
	if q2 := d.Questions[2]; q2.Options[0].ID != "true" || !q2.Options[0].Correct {
		t.Fatalf("tf: %+v", q2)
	}
	if q3 := d.Questions[3]; !reflect.DeepEqual(q3.AcceptedAnswers, []string{"Vell", "Vell Valley"}) {
		t.Fatalf("short: %+v", q3)
	}
	if q4 := d.Questions[4]; q4.Cue != "Zorbs ripen when ..." || q4.SourceDocument != "DEMO reading" || q4.Rubric == "" {
		t.Fatalf("cued: %+v", q4)
	}
	if q5 := d.Questions[5]; q5.Points != 5 || q5.Prompt != "Write everything you remember.\nIt can span lines." {
		t.Fatalf("free: %+v", q5)
	}
}

func TestParseRejectsInvalidQuizzesWithUsefulErrors(t *testing.T) {
	cases := map[string]struct{ name, body, want string }{
		"no title":        {"q.yaml", "questions: [{type: free_recall, prompt: x}]", "title"},
		"no questions":    {"q.yaml", "title: x", "at least one question"},
		"bad type":        {"q.yaml", "title: x\nquestions: [{type: essay, prompt: x}]", "question 1"},
		"no correct":      {"q.yaml", "title: x\nquestions: [{type: single_choice, prompt: x, options: [a, b]}]", "correct"},
		"two correct":     {"q.yaml", "title: x\nquestions: [{type: single_choice, prompt: x, options: ['*a', '*b']}]", "exactly one"},
		"one option":      {"q.yaml", "title: x\nquestions: [{type: multiple_choice, prompt: x, options: ['*a']}]", "two options"},
		"empty prompt":    {"q.yaml", "title: x\nquestions: [{type: free_recall, prompt: ' '}]", "prompt"},
		"naive date":      {"q.yaml", "title: x\nopen_at: '2026-10-10 09:00'\nquestions: [{type: free_recall, prompt: x}]", "time zone"},
		"window order":    {"q.yaml", "title: x\nopen_at: '2026-10-10T09:00:00Z'\nclose_at: '2026-10-09T09:00:00Z'\nquestions: [{type: free_recall, prompt: x}]", "close_at"},
		"bad audience":    {"q.yaml", "title: x\naudience: everyone\nquestions: [{type: free_recall, prompt: x}]", "audience"},
		"bad kind":        {"q.yaml", "title: x\nkind: exam\nquestions: [{type: free_recall, prompt: x}]", "kind"},
		"negative points": {"q.yaml", "title: x\nquestions: [{type: free_recall, prompt: x, points: -1}]", "points"},
		"unknown field":   {"q.yaml", "title: x\nshufle: true\nquestions: [{type: free_recall, prompt: x}]", "shufle"},
		"md no title":     {"q.md", "## free_recall\nx", "title"},
		"md bad heading":  {"q.md", "# T\n## essay\nx", "line 2"},
		"tf no answer":    {"q.yaml", "title: x\nquestions: [{type: true_false, prompt: x}]", "answer"},
		"bad anchor":      {"q.yaml", "title: x\nafter: {anchor: lunch, days: 1}\nquestions: [{type: free_recall, prompt: x}]", "anchor"},
	}
	for name, c := range cases {
		_, err := Parse(c.name, []byte(c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v does not mention %q", name, err, c.want)
		}
	}
}

func TestExportRoundTrips(t *testing.T) {
	d, err := Parse("quiz.yaml", []byte(yamlQuiz))
	if err != nil {
		t.Fatal(err)
	}
	out, err := ExportYAML(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse("again.yaml", out)
	if err != nil {
		t.Fatalf("re-import: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(stripIDs(d), stripIDs(back)) {
		t.Fatalf("round trip changed quiz:\n%+v\n%+v\n%s", d, back, out)
	}
}

func stripIDs(d Draft) Draft {
	for i := range d.Questions {
		for j := range d.Questions[i].Options {
			d.Questions[i].Options[j].ID = ""
		}
	}
	if d.Settings.OpenAt != nil {
		u := d.Settings.OpenAt.UTC()
		d.Settings.OpenAt = &u
	}
	return d
}
