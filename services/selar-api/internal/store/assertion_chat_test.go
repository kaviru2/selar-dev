package store

import (
	"context"
	"github.com/selar-dev/selar-api/internal/model"
	"testing"
)

func TestSavedAssertionAnswerRatingNeverAdaptsGraph(t *testing.T) {
	f, done := newRollbackFixture(t)
	defer done()
	ctx := context.Background()
	message := f.answer(t)
	if _, err := f.pool.Exec(ctx, `UPDATE chat_messages SET model_version='deterministic-saved-assertion-v1' WHERE id=$1`, message); err != nil {
		t.Fatal(err)
	}
	before := f.state(t)
	for _, action := range []string{"helpful", "unhelpful", "correction"} {
		if _, err := f.s.RecordChatFeedback(ctx, f.owner, message, model.ChatFeedbackRequest{Action: action, CorrectionText: "Saved-record feedback only"}); err != nil {
			t.Fatal(err)
		}
		if after := f.state(t); after != before {
			t.Fatalf("%s mutated graph: before=%+v after=%+v", action, before, after)
		}
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM chat_graph_updates WHERE message_id=$1`, message).Scan(&count); err != nil || count != 0 {
		t.Fatalf("graph update recorded: %d %v", count, err)
	}
}
