// suggestion.go — Link suggestion domain model.
// Represents AI-identified semantic links between chunks across documents.
// Each suggestion has a similarity score, relation type, and user response status.
// This is THE core table for the pedagogical intervention study.
package model

import (
	"encoding/json"
	"time"
)

// SuggestionStatus represents the state of a semantic match suggestion.
type SuggestionStatus string

const (
	SuggestionPending   SuggestionStatus = "pending"
	SuggestionConfirmed SuggestionStatus = "confirmed"
	SuggestionRejected  SuggestionStatus = "rejected"
	SuggestionRelabeled SuggestionStatus = "relabeled"
	SuggestionExpired   SuggestionStatus = "expired"
)

// RelationType labels the semantic relationship between two chunks.
type RelationType string

const (
	RelationRelatedTo      RelationType = "related_to"
	RelationPrerequisiteOf RelationType = "prerequisite_of"
	RelationSubConceptOf   RelationType = "sub_concept_of"
	RelationContradicts    RelationType = "contradicts"
	RelationExtends        RelationType = "extends"
)

// LinkSuggestion represents an AI-identified semantic link between two chunks.
type LinkSuggestion struct {
	ID              string           `json:"id"`
	UserID          string           `json:"user_id"`
	SourceChunkID   string           `json:"source_chunk_id"`
	TargetChunkID   string           `json:"target_chunk_id"`
	Similarity      float32          `json:"similarity"`
	Relation        RelationType     `json:"relation"`
	Status          SuggestionStatus `json:"status"`
	UserLabel       *string          `json:"user_label,omitempty"`
	TimeToRespondMs *int             `json:"time_to_respond_ms,omitempty"`
	SuggestedAt     time.Time        `json:"suggested_at"`
	RespondedAt     *time.Time       `json:"responded_at,omitempty"`

	// Joined fields for API responses
	SrcText     string          `json:"src_text,omitempty"`
	TgtText     string          `json:"tgt_text,omitempty"`
	SrcDoc      string          `json:"src_doc,omitempty"`
	TgtDoc      string          `json:"tgt_doc,omitempty"`
	TgtPage     int             `json:"tgt_page,omitempty"`
	SrcBBoxes   json.RawMessage `json:"src_bboxes,omitempty"`
}

// SuggestionResponse is the payload for confirming/rejecting a suggestion.
type SuggestionResponse struct {
	Action          string `json:"action"`
	Label           string `json:"label,omitempty"`
	TimeToRespondMs int    `json:"time_to_respond_ms"`
}
