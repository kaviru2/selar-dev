package store

import (
	"github.com/selar-dev/selar-api/internal/model"
	"testing"
)

func TestGroundedReviewHidesSupersededModelVersion(t *testing.T) {
	s, ctx, owner, _, id, doc, cleanup := reviewFixture(t)
	defer cleanup()
	zero := int64(0)
	if err := s.RespondToMentalModelLink(ctx, owner, id, model.MentalModelLinkResponse{Action: model.MentalLinkConfirmed, Revision: &zero}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO document_mental_models(user_id,document_id,version,main_claim,key_concepts) VALUES ($1,$2,2,'new extraction',ARRAY['gradient descent optimization'])`, owner, doc); err != nil {
		t.Fatal(err)
	}
	if p, err := s.PreviewMentalModelLink(ctx, owner, id); err != ErrMentalModelLinkNotFound {
		t.Fatalf("old version still previewable: %+v %v", p, err)
	}
	edges, err := s.ListReviewedMentalLinkEdges(ctx, owner)
	if err != nil || len(edges) != 0 {
		t.Fatalf("orphan graph edge: %+v %v", edges, err)
	}
	one := int64(1)
	if err := s.RespondToMentalModelLink(ctx, owner, id, model.MentalModelLinkResponse{Action: "retracted", Revision: &one, Reason: "stale version"}); err != ErrMentalModelLinkNotFound {
		t.Fatalf("old model version accepted: %v", err)
	}
}
