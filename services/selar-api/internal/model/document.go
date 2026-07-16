// document.go — Document domain model.
// Represents uploaded PDF documents with processing status tracking
// and aggregate statistics for the library dashboard view.
package model

import "time"

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
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Title        string     `json:"title"`
	Authors      string     `json:"authors,omitempty"`
	Year         int        `json:"year,omitempty"`
	PageCount    int        `json:"page_count,omitempty"`
	Status       DocStatus  `json:"status"`
	Progress     float64    `json:"progress,omitempty"`
	FilePath     string     `json:"-"`
	GDriveFileID string     `json:"-"`
	AddedAt      time.Time  `json:"added_at"`
	ProcessedAt  *time.Time `json:"processed_at,omitempty"`
}

// DocumentStats holds aggregate stats for a user's library.
type DocumentStats struct {
	TotalDocuments int     `json:"total_documents"`
	TotalChunks    int     `json:"total_chunks"`
	ConfirmedLinks int     `json:"confirmed_links"`
	ReadingTimeMin float64 `json:"reading_time_min"`
}
