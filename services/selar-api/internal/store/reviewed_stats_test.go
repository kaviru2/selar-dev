package store

import (
	"testing"

	"github.com/selar-dev/selar-api/internal/model"
)

// The Library "Links you kept" stat must count the grounded links a learner
// keeps in the Reader (mental_model_links), owner-scoped, and drop them again
// when the learner retracts. It used to read link_suggestions and was always 0.
func TestGroundedReviewKeptLinksCountInLibraryStats(t *testing.T) {
	s, ctx, owner, intruder, id, _, cleanup := reviewFixture(t)
	defer cleanup()
	rev := func(n int64) *int64 { return &n }
	kept := func(user string) int {
		t.Helper()
		stats, err := s.GetDocumentStats(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		return stats.ConfirmedLinks
	}
	if got := kept(owner); got != 0 {
		t.Fatalf("unreviewed candidate counted as kept: %d", got)
	}
	if err := s.RespondToMentalModelLink(ctx, owner, id, model.MentalModelLinkResponse{Action: model.MentalLinkConfirmed, Revision: rev(0)}); err != nil {
		t.Fatal(err)
	}
	if got := kept(owner); got != 1 {
		t.Fatalf("kept link not counted: %d", got)
	}
	if got := kept(intruder); got != 0 {
		t.Fatalf("another learner's kept link leaked into stats: %d", got)
	}
	if err := s.RespondToMentalModelLink(ctx, owner, id, model.MentalModelLinkResponse{Action: model.MentalLinkRelabeled, Revision: rev(1), Label: "Compare optimization methods"}); err != nil {
		t.Fatal(err)
	}
	if got := kept(owner); got != 1 {
		t.Fatalf("relabeled kept link not counted: %d", got)
	}
	if err := s.RespondToMentalModelLink(ctx, owner, id, model.MentalModelLinkResponse{Action: "retracted", Revision: rev(2), Reason: "no longer endorse"}); err != nil {
		t.Fatal(err)
	}
	if got := kept(owner); got != 0 {
		t.Fatalf("retracted link still counted: %d", got)
	}
}
