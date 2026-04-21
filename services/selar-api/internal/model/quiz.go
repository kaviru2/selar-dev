// quiz.go — Retention test domain models.
// Covers quizzes (pre, post, delayed phases), questions (MCQ, short answer, cloze),
// attempts, and individual responses with timing data for per-concept analysis.
package model

import "time"

// QuizPhase identifies when the quiz is administered.
type QuizPhase string

const (
	QuizPhasePre     QuizPhase = "pre"
	QuizPhasePost    QuizPhase = "post"
	QuizPhaseDelayed QuizPhase = "delayed"
)

// Quiz represents a retention test.
type Quiz struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Phase       QuizPhase `json:"phase"`
	CorpusTag   string    `json:"corpus_tag,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// QuestionType identifies the format of a quiz question.
type QuestionType string

const (
	QuestionMCQ         QuestionType = "mcq"
	QuestionShortAnswer QuestionType = "short_answer"
	QuestionCloze       QuestionType = "cloze"
	QuestionConnection  QuestionType = "connection"
)

// QuizQuestion represents a single question in a quiz.
type QuizQuestion struct {
	ID            string       `json:"id"`
	QuizID        string       `json:"quiz_id"`
	QuestionIndex int          `json:"question_index"`
	Type          QuestionType `json:"type"`
	Prompt        string       `json:"prompt"`
	Options       any          `json:"options,omitempty"`
	CorrectAnswer string       `json:"-"`
	ConceptTags   []string     `json:"concept_tags,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

// QuizAttempt tracks a user's attempt at a quiz.
type QuizAttempt struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	QuizID      string     `json:"quiz_id"`
	StartedAt   time.Time  `json:"started_at"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	Score       *float64   `json:"score,omitempty"`
}

// QuizResponse records a single answer in an attempt.
type QuizResponse struct {
	ID              string `json:"id"`
	AttemptID       string `json:"attempt_id"`
	QuestionID      string `json:"question_id"`
	UserAnswer      string `json:"user_answer,omitempty"`
	IsCorrect       *bool  `json:"is_correct,omitempty"`
	TimeToAnswerMs  int    `json:"time_to_answer_ms"`
}
