// concept.go — Concept and ConceptEdge domain models.
// Concepts are the "nodes" in the lightweight knowledge graph.
// Edges represent confirmed relationships between concepts, replacing Neo4j.
package model

import "time"

// Concept represents a named concept extracted from document chunks.
type Concept struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	State         string    `json:"state"`
	ModelVersion  string    `json:"model_version,omitempty"`
	PromptVersion string    `json:"prompt_version,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ChunkConcept links chunks to the concepts they mention.
type ChunkConcept struct {
	ChunkID    string  `json:"chunk_id"`
	ConceptID  string  `json:"concept_id"`
	Confidence float32 `json:"confidence"`
}

// EdgeCreatedVia records how a concept edge was created.
type EdgeCreatedVia string

const (
	EdgeAISuggested       EdgeCreatedVia = "ai_suggested"
	EdgeUserConfirmed     EdgeCreatedVia = "user_confirmed"
	EdgeUserCreated       EdgeCreatedVia = "user_created"
	EdgeDeterministicChat EdgeCreatedVia = "deterministic_chat"
)

// ConceptEdge represents a directed relationship between two concepts.
type ConceptEdge struct {
	ID                 string         `json:"id"`
	UserID             string         `json:"user_id"`
	SourceConceptID    string         `json:"source_concept_id"`
	TargetConceptID    string         `json:"target_concept_id"`
	Relation           RelationType   `json:"relation"`
	CreatedVia         EdgeCreatedVia `json:"created_via"`
	State              string         `json:"state"`
	Confidence         float32        `json:"confidence"`
	SupportCount       int            `json:"support_count,omitempty"`
	DocumentCount      int            `json:"document_count,omitempty"`
	BaseConfidence     float32        `json:"base_confidence,omitempty"`
	EvidenceConfidence float32        `json:"evidence_confidence,omitempty"`
	ConfirmedAt        *time.Time     `json:"confirmed_at,omitempty"`
	ValidFrom          time.Time      `json:"valid_from"`
	ValidTo            *time.Time     `json:"valid_to,omitempty"`
	ObservedAt         time.Time      `json:"observed_at"`
	SupersededBy       string         `json:"superseded_by,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}
