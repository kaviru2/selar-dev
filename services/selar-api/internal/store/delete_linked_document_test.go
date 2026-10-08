package store

import (
	"testing"

	"github.com/selar-dev/selar-api/internal/model"
)

// Deleting a document must cascade through grounded mental-model links, even
// after the link was reviewed. The cascade nulls the evidence chunk ids
// (ON DELETE SET NULL) before the link row itself is removed, and that
// intermediate UPDATE must not be rejected by the grounding/evidence triggers.
func TestDeleteDocumentWithReviewedGroundedLinkCascades(t *testing.T) {
	s, ctx, owner, _, id, tgtDoc, cleanup := reviewFixture(t)
	defer cleanup()
	if err := s.RespondToMentalModelLink(ctx, owner, id, model.MentalModelLinkResponse{Action: model.MentalLinkConfirmed, Revision: ptrInt64(0)}); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := s.DeleteDocument(ctx, tgtDoc, owner); err != nil {
		t.Fatalf("delete document with a reviewed link: %v", err)
	}
	var docs, links, events int
	if err := s.pool.QueryRow(ctx, `SELECT
		 (SELECT count(*) FROM documents WHERE id=$1),
		 (SELECT count(*) FROM mental_model_links WHERE id=$2),
		 (SELECT count(*) FROM mental_link_review_events WHERE link_id=$2)`, tgtDoc, id).Scan(&docs, &links, &events); err != nil {
		t.Fatal(err)
	}
	if docs != 0 || links != 0 || events != 0 {
		t.Fatalf("after delete: docs=%d links=%d events=%d, want all 0", docs, links, events)
	}
}

func TestDeleteDocumentWithCandidateGroundedLinkCascades(t *testing.T) {
	s, ctx, owner, _, id, tgtDoc, cleanup := reviewFixture(t)
	defer cleanup()
	if err := s.DeleteDocument(ctx, tgtDoc, owner); err != nil {
		t.Fatalf("delete document with a candidate link: %v", err)
	}
	var links int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mental_model_links WHERE id=$1`, id).Scan(&links); err != nil || links != 0 {
		t.Fatalf("link survived document deletion: %d %v", links, err)
	}
}

func ptrInt64(v int64) *int64 { return &v }
