package model

import (
	"encoding/json"
	"time"
)

// MentalModelStatus describes the lifecycle of an extracted document model.
type MentalModelStatus string

const (
	MentalModelDraft      MentalModelStatus = "draft"
	MentalModelReady      MentalModelStatus = "ready"
	MentalModelFailed     MentalModelStatus = "failed"
	MentalModelSuperseded MentalModelStatus = "superseded"
)

// MentalLinkType is one of the four relationships evaluated by the study.
type MentalLinkType string

const (
	MentalLinkConceptOverlap     MentalLinkType = "concept_overlap"
	MentalLinkClaimExtension     MentalLinkType = "claim_extension"
	MentalLinkAssumptionConflict MentalLinkType = "assumption_conflict"
	MentalLinkQuestionResolution MentalLinkType = "question_resolution"
)

// MentalLinkStatus governs candidate promotion without deleting history.
type MentalLinkStatus string

const (
	MentalLinkCandidate MentalLinkStatus = "candidate"
	MentalLinkConfirmed MentalLinkStatus = "confirmed"
	MentalLinkRejected  MentalLinkStatus = "rejected"
	MentalLinkRelabeled MentalLinkStatus = "relabeled"
	MentalLinkArchived  MentalLinkStatus = "archived"
)

// DocumentMentalModel is the structured argument-level representation of a reading.
type DocumentMentalModel struct {
	ID            string            `json:"id"`
	DocumentID    string            `json:"document_id"`
	UserID        string            `json:"user_id"`
	DocumentTitle string            `json:"document_title,omitempty"`
	Version       int               `json:"version"`
	MainClaim     string            `json:"main_claim"`
	KeyConcepts   []string          `json:"key_concepts"`
	Assumptions   []string          `json:"assumptions"`
	OpenQuestions []string          `json:"open_questions"`
	Domain        string            `json:"domain"`
	ModelVersion  string            `json:"model_version"`
	PromptVersion string            `json:"prompt_version"`
	Status        MentalModelStatus `json:"status"`
	GeneratedAt   time.Time         `json:"generated_at"`
}

// MentalModelLink is an evidence-backed candidate connection between readings.
type MentalModelLink struct {
	ID                    string           `json:"id"`
	UserID                string           `json:"user_id"`
	SourceModelID         string           `json:"source_model_id"`
	TargetModelID         string           `json:"target_model_id"`
	SourceDocumentID      string           `json:"source_document_id"`
	TargetDocumentID      string           `json:"target_document_id"`
	SourceDocumentTitle   string           `json:"source_document_title"`
	TargetDocumentTitle   string           `json:"target_document_title"`
	LinkType              MentalLinkType   `json:"link_type"`
	Similarity            float32          `json:"similarity"`
	Confidence            float32          `json:"confidence"`
	BridgeExplanation     string           `json:"bridge_explanation"`
	SourceEvidenceChunkID *string          `json:"source_evidence_chunk_id,omitempty"`
	TargetEvidenceChunkID *string          `json:"target_evidence_chunk_id,omitempty"`
	SourceEvidence        string           `json:"source_evidence,omitempty"`
	TargetEvidence        string           `json:"target_evidence,omitempty"`
	Status                MentalLinkStatus `json:"status"`
	CreatedVia            EdgeCreatedVia   `json:"created_via"`
	ModelVersion          string           `json:"model_version"`
	PromptVersion         string           `json:"prompt_version"`
	UserLabel             *string          `json:"user_label,omitempty"`
	SuggestedAt           time.Time        `json:"suggested_at"`
	RespondedAt           *time.Time       `json:"responded_at,omitempty"`
}

// MentalModelLinkResponse records a human-in-the-loop candidate decision.
type MentalModelLinkResponse struct {
	Action MentalLinkStatus `json:"action"`
	Label  string           `json:"label,omitempty"`
}

// LearningEvent is an immutable observation used by deterministic reducers.
type LearningEvent struct {
	ID             string          `json:"id"`
	UserID         string          `json:"user_id"`
	EventType      string          `json:"event_type"`
	OccurredAt     time.Time       `json:"occurred_at"`
	DocumentID     *string         `json:"document_id,omitempty"`
	ChunkID        *string         `json:"chunk_id,omitempty"`
	ConceptID      *string         `json:"concept_id,omitempty"`
	SuggestionID   *string         `json:"suggestion_id,omitempty"`
	MentalLinkID   *string         `json:"mental_link_id,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	Source         string          `json:"source"`
	SchemaVersion  int             `json:"schema_version"`
	IdempotencyKey string          `json:"idempotency_key"`
}

// LearnerConceptState is a replayable materialized estimate, not graph truth.
type LearnerConceptState struct {
	UserID            string     `json:"user_id"`
	ConceptID         string     `json:"concept_id"`
	ConceptName       string     `json:"concept_name"`
	MasteryEstimate   float32    `json:"mastery_estimate"`
	RecallProbability float64    `json:"recall_probability"`
	HalfLifeSeconds   float64    `json:"half_life_seconds"`
	LastExposedAt     *time.Time `json:"last_exposed_at,omitempty"`
	LastRetrievedAt   *time.Time `json:"last_retrieved_at,omitempty"`
	SuccessCount      int        `json:"success_count"`
	FailureCount      int        `json:"failure_count"`
	EvidenceCount     int        `json:"evidence_count"`
	Uncertainty       float32    `json:"uncertainty"`
	StateVersion      int        `json:"state_version"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// GraphNode is a unified projection for concepts and mental-model elements.
type GraphNode struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	NodeType      string    `json:"node_type"`
	State         string    `json:"state"`
	DocumentID    string    `json:"document_id,omitempty"`
	DocumentTitle string    `json:"document_title,omitempty"`
	Confidence    float32   `json:"confidence,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// GraphEdge is a unified projection across semantic and argument-level links.
type GraphEdge struct {
	ID          string  `json:"id"`
	Source      string  `json:"source"`
	Target      string  `json:"target"`
	Relation    string  `json:"relation"`
	State       string  `json:"state"`
	Confidence  float32 `json:"confidence,omitempty"`
	CreatedVia  string  `json:"created_via"`
	Explanation string  `json:"explanation,omitempty"`
}
