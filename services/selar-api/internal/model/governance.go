package model

import "time"

type ChatFeedback struct {
	ID             string    `json:"id"`
	MessageID      string    `json:"message_id"`
	Action         string    `json:"action"`
	CorrectionText string    `json:"correction_text,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type ChatFeedbackRequest struct {
	Action         string `json:"action"`
	CorrectionText string `json:"correction_text"`
}

type EdgeActionRequest struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type LearnerSignalRequest struct {
	ConceptID     string `json:"concept_id"`
	Signal        string `json:"signal"`
	IdempotencyID string `json:"idempotency_id"`
}

type ReplayRequest struct {
	Apply bool `json:"apply"`
}

type ReplayReport struct {
	ReducerVersion string    `json:"reducer_version"`
	AsOf           time.Time `json:"as_of"`
	Applied        bool      `json:"applied"`
	Differences    int       `json:"differences"`
	ProjectionHash string    `json:"projection_hash"`
	EdgeCount      int       `json:"edge_count"`
	LearnerCount   int       `json:"learner_count"`
	Equivalent     bool      `json:"equivalent"`
}

type MetricsSummary struct {
	ChatTurns          int     `json:"chat_turns"`
	AverageTotalMS     float64 `json:"average_total_ms"`
	AverageRetrievalMS float64 `json:"average_retrieval_ms"`
	AverageCitations   float64 `json:"average_citations"`
	CitationOpens      int     `json:"citation_opens"`
	HelpfulAnswers     int     `json:"helpful_answers"`
	UnhelpfulAnswers   int     `json:"unhelpful_answers"`
	Corrections        int     `json:"corrections"`
	ConfirmedEdges     int     `json:"confirmed_edges"`
	RejectedEdges      int     `json:"rejected_edges"`
}

type ChatMetrics struct {
	EmbeddingMS    float64 `json:"embedding_ms"`
	RetrievalMS    float64 `json:"retrieval_ms"`
	GenerationMS   float64 `json:"generation_ms"`
	TotalMS        float64 `json:"total_ms"`
	CandidateCount int     `json:"candidate_count"`
	ModelCalls     int     `json:"model_calls"`
}
