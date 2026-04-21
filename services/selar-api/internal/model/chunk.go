// chunk.go — Chunk and Annotation domain models.
// Chunks are text segments extracted from PDF pages with bounding-box
// coordinates in PDF points. Annotations are user-created highlights/notes.
// All coordinates use PDF points (1/72 inch), never screen pixels.
package model

import (
	"encoding/json"
	"time"
)

// BBox represents a bounding box in PDF points (1/72 inch).
// Origin is at bottom-left of the page (PostScript convention).
type BBox struct {
	Page int     `json:"page,omitempty"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Unit string  `json:"unit"` // always "pdf_points"
}

// Chunk represents a text chunk extracted from a document.
type Chunk struct {
	ID          string          `json:"id"`
	DocumentID  string          `json:"document_id"`
	UserID      string          `json:"user_id"`
	ChunkIndex  int             `json:"chunk_index"`
	PageStart   int             `json:"page_start"`
	PageEnd     int             `json:"page_end"`
	Content     string          `json:"content"`
	TokenCount  int             `json:"token_count"`
	BBoxes      json.RawMessage `json:"bboxes"`
	CreatedAt   time.Time       `json:"created_at"`
}

// AnnotationType represents the kind of annotation.
type AnnotationType string

const (
	AnnotationHighlight  AnnotationType = "highlight"
	AnnotationUnderline  AnnotationType = "underline"
	AnnotationNote       AnnotationType = "note"
	AnnotationSuggestion AnnotationType = "suggestion"
)

// AnnotationColor constrains the highlight palette.
type AnnotationColor string

const (
	ColorWheat  AnnotationColor = "wheat"
	ColorYellow AnnotationColor = "yellow"
	ColorCoral  AnnotationColor = "coral"
	ColorSage   AnnotationColor = "sage"
)

// Annotation represents a user-created highlight or note on a PDF page.
// Coordinates are always in PDF points for cross-device sync.
type Annotation struct {
	ID         string          `json:"id"`
	UserID     string          `json:"user_id"`
	DocumentID string          `json:"document_id"`
	ChunkID    *string         `json:"chunk_id,omitempty"`
	Page       int             `json:"page"`
	BBox       json.RawMessage `json:"bbox"`
	Color      AnnotationColor `json:"color"`
	Type       AnnotationType  `json:"type"`
	Comment    string          `json:"comment,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}
