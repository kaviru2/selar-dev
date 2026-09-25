package store

import (
	"github.com/selar-dev/selar-api/internal/model"
	"testing"
)

func TestGroundedReviewLifecycleAndCurrentGraphProjection(t *testing.T) {
	s, ctx, owner, intruder, id, doc, cleanup := reviewFixture(t)
	defer cleanup()
	rev := func(n int64) *int64 { return &n }
	decide := func(n int64, action model.MentalLinkStatus, label, reason string, target *int64) error {
		return s.RespondToMentalModelLink(ctx, owner, id, model.MentalModelLinkResponse{Action: action, Revision: rev(n), Label: label, Reason: reason, TargetRevision: target})
	}
	if err := s.RespondToMentalModelLink(ctx, intruder, id, model.MentalModelLinkResponse{Action: model.MentalLinkConfirmed, Revision: rev(0)}); err != ErrMentalModelLinkNotFound {
		t.Fatalf("cross-owner mutation: %v", err)
	}
	if err := decide(0, model.MentalLinkConfirmed, "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := decide(0, model.MentalLinkConfirmed, "", "", nil); err != ErrReviewStale {
		t.Fatalf("stale repeat: %v", err)
	}
	p, err := s.PreviewMentalModelLink(ctx, owner, id)
	if err != nil || p.Revision != 1 || len(p.History) != 1 {
		t.Fatalf("confirm preview: %+v %v", p, err)
	}
	edges, err := s.ListReviewedMentalLinkEdges(ctx, owner)
	if err != nil || len(edges) != 1 || edges[0].MentalLinkID != id || edges[0].SourceQuote == "" || edges[0].ReviewRevision != 1 {
		t.Fatalf("graph projection: %+v %v", edges, err)
	}
	var successes, events int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM learner_concept_state WHERE user_id=$1 AND success_count>0`, owner).Scan(&successes); err != nil || successes != 0 {
		t.Fatalf("review faked retrieval success: %d %v", successes, err)
	}
	if err := decide(1, model.MentalLinkRelabeled, "Compare optimization methods", "human correction", nil); err != nil {
		t.Fatal(err)
	}
	if err := decide(2, "retracted", "", "no longer endorse", nil); err != nil {
		t.Fatal(err)
	}
	edges, err = s.ListReviewedMentalLinkEdges(ctx, owner)
	if err != nil || len(edges) != 0 {
		t.Fatalf("retracted edge visible: %+v %v", edges, err)
	}
	if err := decide(3, "rolled_back", "", "undo latest retraction", rev(2)); err != nil {
		t.Fatal(err)
	}
	edges, err = s.ListReviewedMentalLinkEdges(ctx, owner)
	if err != nil || len(edges) != 1 || edges[0].Relation != "concept_overlap" {
		t.Fatalf("rollback did not restore grounded relation: %+v %v", edges, err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM mental_link_review_events WHERE user_id=$1 AND link_id=$2`, owner, id).Scan(&events); err != nil || events != 4 {
		t.Fatalf("immutable event count: %d %v", events, err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE documents SET content_hash='changed' WHERE id=$1`, doc); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PreviewMentalModelLink(ctx, owner, id); err != ErrMentalModelLinkNotFound {
		t.Fatalf("stale witness preview: %v", err)
	}
	edges, err = s.ListReviewedMentalLinkEdges(ctx, owner)
	if err != nil || len(edges) != 0 {
		t.Fatalf("stale witness graph: %+v %v", edges, err)
	}
	if err := decide(4, "retracted", "", "stale", nil); err != ErrMentalModelLinkNotFound {
		t.Fatalf("stale witness mutation: %v", err)
	}
}
