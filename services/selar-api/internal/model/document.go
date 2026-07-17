// document.go — Document domain model.
// Represents uploaded PDF documents with processing status tracking
// and aggregate statistics for the library dashboard view.
package model

import (
	"encoding/json"
	"time"
)

// DocStatus represents the processing state of a document.
type DocStatus string

const (
	DocStatusUploaded   DocStatus = "uploaded"
	DocStatusProcessing DocStatus = "processing"
	DocStatusReady      DocStatus = "ready"
	DocStatusFailed     DocStatus = "failed"
)

// Document represents an uploaded PDF document.
type Document struct {
	ID           string          `json:"id"`
	UserID       string          `json:"user_id"`
	Title        string          `json:"title"`
	Authors      string          `json:"authors,omitempty"`
	Year         int             `json:"year,omitempty"`
	PageCount    int             `json:"page_count,omitempty"`
	Status       DocStatus       `json:"status"`
	Progress     float64         `json:"progress,omitempty"`
	FilePath     string          `json:"-"`
	GDriveFileID string          `json:"-"`
	SourceID     *string         `json:"source_id,omitempty"`
	SourceType   string          `json:"source_type"`
	SourceURL    string          `json:"source_url,omitempty"`
	CanonicalURL string          `json:"canonical_url,omitempty"`
	ContentHash  string          `json:"content_hash,omitempty"`
	MimeType     string          `json:"mime_type,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	FetchedAt    *time.Time      `json:"fetched_at,omitempty"`
	AddedAt      time.Time       `json:"added_at"`
	ProcessedAt  *time.Time      `json:"processed_at,omitempty"`
}

type ContentBlock struct {
	ID         string          `json:"id"`
	DocumentID string          `json:"document_id"`
	BlockIndex int             `json:"block_index"`
	Kind       string          `json:"kind"`
	Text       string          `json:"text"`
	Locator    json.RawMessage `json:"locator"`
	Metadata   json.RawMessage `json:"metadata"`
}

type Asset struct {
	ID               string          `json:"id"`
	DocumentID       string          `json:"document_id"`
	BlockIndex       *int            `json:"block_index,omitempty"`
	Kind             string          `json:"kind"`
	StoragePath      string          `json:"-"`
	SourceURL        string          `json:"source_url,omitempty"`
	MimeType         string          `json:"mime_type"`
	Width            int             `json:"width"`
	Height           int             `json:"height"`
	ContentHash      string          `json:"content_hash"`
	Caption          string          `json:"caption,omitempty"`
	AltText          string          `json:"alt_text,omitempty"`
	Description      string          `json:"description,omitempty"`
	Locator          json.RawMessage `json:"locator"`
	EmbeddingModel   string          `json:"embedding_model"`
	EmbeddingVersion string          `json:"embedding_version"`
}

type DocumentContent struct {
	Document *Document      `json:"document"`
	Blocks   []ContentBlock `json:"blocks"`
	Assets   []Asset        `json:"assets"`
}

// DocumentStats holds aggregate stats for a user's library.
type DocumentStats struct {
	TotalDocuments int     `json:"total_documents"`
	TotalChunks    int     `json:"total_chunks"`
	ConfirmedLinks int     `json:"confirmed_links"`
	ReadingTimeMin float64 `json:"reading_time_min"`
}
