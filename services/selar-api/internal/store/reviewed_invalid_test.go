package store

import (
	"github.com/selar-dev/selar-api/internal/model"
	"testing"
)

func TestGroundedReviewRejectsMissingAndChangedWitness(t *testing.T) {
	s, ctx, owner, _, id, _, cleanup := reviewFixture(t)
	defer cleanup()
	zero := int64(0)
	request := model.MentalModelLinkResponse{Action: model.MentalLinkConfirmed, Revision: &zero}
	if err := s.RespondToMentalModelLink(ctx, owner, "00000000-0000-0000-0000-000000000000", request); err != ErrMentalModelLinkNotFound {
		t.Fatalf("missing candidate: %v", err)
	}
	// The quote remains in the old witness JSON, but changing its chunk text
	// must invalidate both preview and mutation even if the document hash is unchanged.
	if _, err := s.pool.Exec(ctx, `UPDATE chunks SET content='a completely different passage' WHERE id=(SELECT source_evidence_chunk_id FROM mental_model_links WHERE id=$1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PreviewMentalModelLink(ctx, owner, id); err != ErrMentalModelLinkNotFound {
		t.Fatalf("changed quote preview: %v", err)
	}
	if err := s.RespondToMentalModelLink(ctx, owner, id, request); err != ErrMentalModelLinkNotFound {
		t.Fatalf("changed quote mutation: %v", err)
	}
}
