package model

import (
	"encoding/json"
	"time"
)

type ContentSource struct {
	ID              string          `json:"id"`
	UserID          string          `json:"user_id"`
	Kind            string          `json:"kind"`
	URI             string          `json:"uri,omitempty"`
	CanonicalURI    string          `json:"canonical_uri,omitempty"`
	Title           string          `json:"title,omitempty"`
	RefreshPolicy   string          `json:"refresh_policy"`
	Status          string          `json:"status"`
	Config          json.RawMessage `json:"config"`
	LastContentHash string          `json:"last_content_hash,omitempty"`
	LastFetchedAt   *time.Time      `json:"last_fetched_at,omitempty"`
	LastError       string          `json:"last_error,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type IngestionRun struct {
	ID                 string          `json:"id"`
	SourceID           string          `json:"source_id"`
	DocumentID         string          `json:"document_id"`
	Status             string          `json:"status"`
	Extractor          string          `json:"extractor,omitempty"`
	ExtractorVersion   string          `json:"extractor_version,omitempty"`
	EmbeddingModel     string          `json:"embedding_model,omitempty"`
	EmbeddingDimension int             `json:"embedding_dimension"`
	Metrics            json.RawMessage `json:"metrics"`
	Error              string          `json:"error,omitempty"`
	StartedAt          *time.Time      `json:"started_at,omitempty"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

type IngestionJob struct {
	ID             string     `json:"id"`
	RunID          string     `json:"run_id"`
	SourceID       string     `json:"source_id"`
	DocumentID     string     `json:"document_id"`
	UserID         string     `json:"user_id"`
	SourceType     string     `json:"source_type"`
	FilePath       string     `json:"file_path,omitempty"`
	SourceURL      string     `json:"source_url,omitempty"`
	RawText        string     `json:"-"`
	Title          string     `json:"title,omitempty"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	AvailableAt    time.Time  `json:"available_at"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	WorkerID       string     `json:"worker_id,omitempty"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
