package store

import (
	"context"
	"github.com/selar-dev/selar-api/internal/model"
	"time"
)

// A live projection, not a second edge table. Old unreviewed or legacy links,
// rejected/retracted links, and witnesses stale since review are absent.
func (s *Store) ListReviewedMentalLinkEdges(ctx context.Context, owner string) ([]model.GraphEdge, error) {
	rows, err := s.pool.Query(ctx, `SELECT ml.id,ml.source_model_id,ml.target_model_id,
 sm.document_id,tm.document_id,ml.source_evidence->>'quote',ml.target_evidence->>'quote',
 ml.review_revision,ml.responded_at,ml.status,ml.confidence
 FROM mental_model_links ml
 JOIN document_mental_models sm ON sm.id=ml.source_model_id
 JOIN document_mental_models tm ON tm.id=ml.target_model_id
 JOIN documents sd ON sd.id=sm.document_id
 JOIN documents td ON td.id=tm.document_id
 WHERE ml.user_id=$1 AND ml.review_revision>0
 AND ml.status IN ('confirmed','relabeled') AND ml.link_type='concept_overlap'
 AND sd.status='ready' AND td.status='ready' AND valid_grounded_mental_link(ml)
 AND NOT EXISTS (SELECT 1 FROM document_mental_models newer WHERE newer.document_id=sm.document_id AND newer.version>sm.version AND newer.status='ready')
 AND NOT EXISTS (SELECT 1 FROM document_mental_models newer WHERE newer.document_id=tm.document_id AND newer.version>tm.version AND newer.status='ready')
 ORDER BY ml.responded_at,ml.id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	edges := []model.GraphEdge{}
	for rows.Next() {
		var e model.GraphEdge
		var source, target string
		var observed time.Time
		if err := rows.Scan(&e.MentalLinkID, &source, &target, &e.SourceDocumentID, &e.TargetDocumentID, &e.SourceQuote, &e.TargetQuote, &e.ReviewRevision, &observed, &e.State, &e.Confidence); err != nil {
			return nil, err
		}
		e.ID = "reviewed:" + e.MentalLinkID
		e.Source = "model:" + source
		e.Target = "model:" + target
		e.Relation = "concept_overlap"
		e.CreatedVia = "user_reviewed"
		e.Explanation = "Learner reviewed exact concept overlap in both source passages. Open the reader to inspect the witnesses."
		e.ValidFrom = &observed
		e.ObservedAt = &observed
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// ListCandidateMentalLinkEdges projects machine-proposed grounded concept-overlap
// candidates that still await the learner's review (issue #117). They are
// returned with state "candidate" and created_via "ai_suggested" so clients can
// draw them distinctly from reviewed links; they carry both exact witnesses so
// the learner can compare the sources. Only rows passing the live two-sided
// witness check on the latest ready models of two ready owner documents are
// returned (stale snapshots, rejected, retracted and reviewed rows are absent).
// Similarity-only passage matches (link_suggestions) are never projected.
// CandidateLinkID is set instead of MentalLinkID so no client can mistake a
// candidate for a learner-reviewed assertion.
func (s *Store) ListCandidateMentalLinkEdges(ctx context.Context, owner string) ([]model.GraphEdge, error) {
	rows, err := s.pool.Query(ctx, `SELECT ml.id,ml.source_model_id,ml.target_model_id,
 sm.document_id,tm.document_id,ml.source_evidence->>'quote',ml.target_evidence->>'quote',
 ml.source_evidence->>'asserted_concept',ml.suggested_at,ml.confidence
 FROM mental_model_links ml
 JOIN document_mental_models sm ON sm.id=ml.source_model_id
 JOIN document_mental_models tm ON tm.id=ml.target_model_id
 JOIN documents sd ON sd.id=sm.document_id
 JOIN documents td ON td.id=tm.document_id
 WHERE ml.user_id=$1 AND ml.status='candidate' AND ml.link_type='concept_overlap'
 AND sm.user_id=$1 AND tm.user_id=$1 AND sd.user_id=$1 AND td.user_id=$1
 AND sd.status='ready' AND td.status='ready' AND sd.visible AND td.visible
 AND valid_grounded_mental_link(ml)
 AND NOT EXISTS (SELECT 1 FROM document_mental_models newer WHERE newer.document_id=sm.document_id AND newer.version>sm.version AND newer.status='ready')
 AND NOT EXISTS (SELECT 1 FROM document_mental_models newer WHERE newer.document_id=tm.document_id AND newer.version>tm.version AND newer.status='ready')
 ORDER BY ml.suggested_at,ml.id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	edges := []model.GraphEdge{}
	for rows.Next() {
		var e model.GraphEdge
		var source, target, concept string
		var suggested time.Time
		if err := rows.Scan(&e.CandidateLinkID, &source, &target, &e.SourceDocumentID, &e.TargetDocumentID,
			&e.SourceQuote, &e.TargetQuote, &concept, &suggested, &e.Confidence); err != nil {
			return nil, err
		}
		e.ID = "candidate:" + e.CandidateLinkID
		e.Source = "model:" + source
		e.Target = "model:" + target
		e.Relation = "concept_overlap"
		e.State = "candidate"
		e.CreatedVia = "ai_suggested"
		e.Explanation = "Suggested by SELAR, not reviewed: both readings name \"" + concept +
			"\". Compare the two passages in the Reader and decide whether the link is real."
		e.ObservedAt = &suggested
		edges = append(edges, e)
	}
	return edges, rows.Err()
}
