package model

import "time"

// ChatThread groups a user conversation over the personal research library.
type ChatThread struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatCitation grounds an assistant message in a stored PDF passage.
type ChatCitation struct {
	ID            string  `json:"id,omitempty"`
	MessageID     string  `json:"message_id,omitempty"`
	ChunkID       string  `json:"chunk_id"`
	DocumentID    string  `json:"document_id"`
	DocumentTitle string  `json:"document_title"`
	Page          int     `json:"page"`
	Rank          int     `json:"rank"`
	Score         float32 `json:"score"`
	Quote         string  `json:"quote"`
}

// ChatMessage is an immutable conversation episode.
type ChatMessage struct {
	ID           string         `json:"id"`
	ThreadID     string         `json:"thread_id"`
	UserID       string         `json:"user_id"`
	Role         string         `json:"role"`
	Content      string         `json:"content"`
	Status       string         `json:"status"`
	ModelVersion string         `json:"model_version,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	Citations    []ChatCitation `json:"citations"`
}

// ChatAnswer is returned by the worker after deterministic retrieval.
type ChatAnswer struct {
	Answer        string         `json:"answer"`
	ModelVersion  string         `json:"model_version"`
	RankingPolicy string         `json:"ranking_policy"`
	Citations     []ChatCitation `json:"citations"`
	Candidates    []any          `json:"candidates"`
}

// ChatMessageRequest creates a new user episode.
type ChatMessageRequest struct {
	Content string `json:"content"`
}
