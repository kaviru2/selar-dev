// session.go — Reading session domain model.
// Tracks user reading behavior: which pages were viewed, session duration,
// and scroll depth. Used for both UX analytics and thesis analysis.
package model

import "time"

// ReadingSession tracks a contiguous reading session for a document.
type ReadingSession struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	DocumentID     string    `json:"document_id"`
	StartedAt      time.Time `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	PagesViewed    []int     `json:"pages_viewed"`
	MaxScrollDepth int       `json:"max_scroll_depth"`
}
