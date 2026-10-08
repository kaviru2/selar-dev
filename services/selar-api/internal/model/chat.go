package model

import (
	"encoding/json"
	"time"
)

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
	ID            string          `json:"id,omitempty"`
	MessageID     string          `json:"message_id,omitempty"`
	ChunkID       string          `json:"chunk_id"`
	DocumentID    string          `json:"document_id"`
	DocumentTitle string          `json:"document_title"`
	Page          int             `json:"page"`
	Rank          int             `json:"rank"`
	Score         float32         `json:"score"`
	Quote         string          `json:"quote"`
	SourceType    string          `json:"source_type"`
	Locator       json.RawMessage `json:"locator"`
}

// ChatMessage is an immutable conversation episode.
type ChatMessage struct {
	ID                  string           `json:"id"`
	ThreadID            string           `json:"thread_id"`
	UserID              string           `json:"user_id"`
	Role                string           `json:"role"`
	Content             string           `json:"content"`
	Status              string           `json:"status"`
	ModelVersion        string           `json:"model_version,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	Citations           []ChatCitation   `json:"citations"`
	GraphUpdate         *ChatGraphUpdate `json:"graph_update,omitempty"`
	Feedback            []ChatFeedback   `json:"feedback,omitempty"`
	SupersedesMessageID string           `json:"supersedes_message_id,omitempty"`
}

// ChatGraphUpdate summarizes deterministic graph changes caused by grounded evidence.
type ChatGraphUpdate struct {
	ConceptsCreated    int    `json:"concepts_created"`
	ConceptsReinforced int    `json:"concepts_reinforced"`
	LinksObserved      int    `json:"links_observed"`
	LinksPromoted      int    `json:"links_promoted"`
	ReducerVersion     string `json:"reducer_version"`
	// Retracted is true once negative feedback or a correction deactivated
	// this answer's adaptive evidence; counts remain for audit only.
	Retracted bool `json:"retracted"`
}

// ChatAnswer is returned by the worker after deterministic retrieval.
type ChatAnswer struct {
	Answer        string         `json:"answer"`
	ModelVersion  string         `json:"model_version"`
	RankingPolicy string         `json:"ranking_policy"`
	Citations     []ChatCitation `json:"citations"`
	Candidates    []any          `json:"candidates"`
	Metrics       ChatMetrics    `json:"metrics"`
}

// ChatMessageRequest creates a new user episode.
type ChatMessageRequest struct {
	Content string `json:"content"`
}
