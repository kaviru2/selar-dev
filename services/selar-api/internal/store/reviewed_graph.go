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
 ml.review_revision,ml.responded_at
 FROM mental_model_links ml
 JOIN document_mental_models sm ON sm.id=ml.source_model_id
 JOIN document_mental_models tm ON tm.id=ml.target_model_id
 JOIN documents sd ON sd.id=sm.document_id
 JOIN documents td ON td.id=tm.document_id
 WHERE ml.user_id=$1 AND ml.review_revision>0
 AND ml.status IN ('confirmed','relabeled') AND ml.link_type='concept_overlap'
 AND sd.status='ready' AND td.status='ready' AND valid_grounded_mental_link(ml)
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
		if err := rows.Scan(&e.MentalLinkID, &source, &target, &e.SourceDocumentID, &e.TargetDocumentID, &e.SourceQuote, &e.TargetQuote, &e.ReviewRevision, &observed); err != nil {
			return nil, err
		}
		e.ID = "reviewed:" + e.MentalLinkID
		e.Source = "model:" + source
		e.Target = "model:" + target
		e.Relation = "concept_overlap"
		e.State = "confirmed"
		e.CreatedVia = "user_reviewed"
		e.Confidence = 1
		e.Explanation = "Learner reviewed exact concept overlap in both source passages. Open the reader to inspect the witnesses."
		e.ValidFrom = &observed
		e.ObservedAt = &observed
		edges = append(edges, e)
	}
	return edges, rows.Err()
}
