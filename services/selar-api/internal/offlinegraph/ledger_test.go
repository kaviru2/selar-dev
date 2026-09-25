package offlinegraph

import (
	"errors"
	"reflect"
	"testing"
)

func sample(id, relation string) Assertion {
	return Assertion{ID: id, SourceID: "library-a", From: "concept-a", Relation: relation, To: "concept-b", FromSupport: Support{DocumentID: "doc-a", ChunkID: "chunk-a"}, ToSupport: Support{DocumentID: "doc-b", ChunkID: "chunk-b"}}
}

func TestCorrectionRequiresPreviewAndExplicitConfirmation(t *testing.T) {
	ledger, err := NewLedger([]Assertion{sample("original", "overlaps_with")})
	if err != nil {
		t.Fatal(err)
	}
	before := ledger.Snapshot()
	preview, err := ledger.PreviewCorrection("original", sample("replacement", "contradicts"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, ledger.Snapshot()) {
		t.Fatal("preview mutated state")
	}
	if preview.Before.ID != "original" || preview.After.ID != "replacement" || preview.BaseRevision != before.Revision {
		t.Fatalf("incorrect preview: %+v", preview)
	}
	if err := ledger.ConfirmCorrection(preview, before.Revision); err != nil {
		t.Fatal(err)
	}
	after := ledger.Snapshot()
	if after.Revision != before.Revision+1 || len(after.Active) != 1 || after.Active[0].ID != "replacement" || len(after.History) != 1 || after.History[0].Kind != Corrected {
		t.Fatalf("incorrect correction: %+v", after)
	}
	if !reflect.DeepEqual(before.Active, []Assertion{sample("original", "overlaps_with")}) {
		t.Fatal("source assertion was rewritten")
	}
	if err := ledger.ConfirmCorrection(preview, before.Revision); !errors.Is(err, ErrStale) {
		t.Fatalf("expected stale preview, got %v", err)
	}
}

func TestRetractionAuditsReasonWithoutErasingSourceOrHistory(t *testing.T) {
	original := sample("original", "overlaps_with")
	ledger, err := NewLedger([]Assertion{original})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Retract("original", 0, "unsupported relation"); err != nil {
		t.Fatal(err)
	}
	s := ledger.Snapshot()
	if s.Revision != 1 || len(s.Active) != 0 || len(s.History) != 1 || s.History[0].Kind != Retracted || s.History[0].Before != original || s.History[0].Reason != "unsupported relation" {
		t.Fatalf("retraction lost audit: %+v", s)
	}
	if err := ledger.Retract("original", 1, "again"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected inactive: %v", err)
	}
	if err := ledger.Retract("original", 0, "stale"); !errors.Is(err, ErrStale) {
		t.Fatalf("expected stale: %v", err)
	}
	if !reflect.DeepEqual(s, ledger.Snapshot()) {
		t.Fatal("rejected retraction mutated state")
	}
}

func TestRollbackReplaysProjectionAndRetainsImmutableHistory(t *testing.T) {
	ledger, err := NewLedger([]Assertion{sample("original", "overlaps_with")})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := ledger.PreviewCorrection("original", sample("replacement", "extends"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.ConfirmCorrection(preview, 0); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Retract("replacement", 1, "bad evidence"); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Rollback(1, 2, "restore reviewed version"); err != nil {
		t.Fatal(err)
	}
	s := ledger.Snapshot()
	if s.Revision != 3 || !reflect.DeepEqual(s.Active, []Assertion{sample("replacement", "extends")}) || len(s.History) != 3 || s.History[2].Kind != RolledBack || s.History[2].TargetRevision != 1 || s.History[2].Reason != "restore reviewed version" {
		t.Fatalf("rollback lost audit or projection: %+v", s)
	}
	if got, err := ledger.SnapshotAt(2); err != nil || len(got.Active) != 0 || len(got.History) != 2 {
		t.Fatalf("prior retraction changed: %+v %v", got, err)
	}
	if err := ledger.Rollback(0, 3, "restore original"); err != nil {
		t.Fatal(err)
	}
	if got, err := ledger.SnapshotAt(4); err != nil || !reflect.DeepEqual(got.Active, []Assertion{sample("original", "overlaps_with")}) || len(got.History) != 4 {
		t.Fatalf("original not restored: %+v %v", got, err)
	}
	if err := ledger.Rollback(7, 4, "future"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("future accepted: %v", err)
	}
	if err := ledger.Rollback(0, 3, "stale"); !errors.Is(err, ErrStale) {
		t.Fatalf("stale accepted: %v", err)
	}
	if ledger.Snapshot().Revision != 4 {
		t.Fatal("rejected rollback changed revision")
	}
}

func TestForgedOrModifiedPreviewCannotBeConfirmed(t *testing.T) {
	ledger, err := NewLedger([]Assertion{sample("a", "extends")})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := ledger.PreviewCorrection("a", sample("b", "contradicts"))
	if err != nil {
		t.Fatal(err)
	}
	forged := Preview{BaseRevision: 0, Before: sample("a", "extends"), After: sample("b", "contradicts")}
	if err := ledger.ConfirmCorrection(forged, 0); !errors.Is(err, ErrInvalidPreview) {
		t.Fatalf("forged preview accepted: %v", err)
	}
	modified := preview
	modified.After.ToSupport.ChunkID = "different"
	if err := ledger.ConfirmCorrection(modified, 0); !errors.Is(err, ErrInvalidPreview) {
		t.Fatalf("modified preview accepted: %v", err)
	}
	if ledger.Snapshot().Revision != 0 {
		t.Fatal("rejected preview changed state")
	}
	if err := ledger.ConfirmCorrection(preview, 0); err != nil {
		t.Fatal(err)
	}
}

func TestSourceScopedSupportsAndRejectedActions(t *testing.T) {
	invalid := sample("a", "extends")
	invalid.ToSupport.ChunkID = ""
	if _, err := NewLedger([]Assertion{invalid}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing target support accepted: %v", err)
	}
	invalid = sample("a", "extends")
	invalid.FromSupport.DocumentID = ""
	if _, err := NewLedger([]Assertion{invalid}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing source document accepted: %v", err)
	}
	if _, err := NewLedger([]Assertion{sample("a", "extends"), sample("a", "overlaps_with")}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate ID accepted: %v", err)
	}
	ledger, err := NewLedger([]Assertion{sample("a", "extends")})
	if err != nil {
		t.Fatal(err)
	}
	crossScope := sample("b", "contradicts")
	crossScope.SourceID = "someone-else"
	if _, err := ledger.PreviewCorrection("a", crossScope); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-scope correction accepted: %v", err)
	}
	missing := sample("b", "contradicts")
	missing.FromSupport.ChunkID = ""
	if _, err := ledger.PreviewCorrection("a", missing); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing source support accepted: %v", err)
	}
	if _, err := ledger.PreviewCorrection("missing", sample("b", "extends")); !errors.Is(err, ErrNotActive) {
		t.Fatalf("missing assertion accepted: %v", err)
	}
	if _, err := ledger.PreviewCorrection("a", sample("a", "extends")); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("reused ID accepted: %v", err)
	}
	if err := ledger.Retract("a", 0, " "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty reason accepted: %v", err)
	}
	if ledger.Snapshot().Revision != 0 {
		t.Fatal("invalid actions mutated ledger")
	}
}

func TestStalePreviewAndHistoricalIDsCannotBeReused(t *testing.T) {
	ledger, err := NewLedger([]Assertion{sample("a", "extends"), sample("other", "extends")})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := ledger.PreviewCorrection("a", sample("b", "contradicts"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Retract("other", 0, "withdrawn"); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ConfirmCorrection(preview, 0); !errors.Is(err, ErrStale) {
		t.Fatalf("stale preview accepted: %v", err)
	}
	fresh, err := ledger.PreviewCorrection("a", sample("b", "contradicts"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.ConfirmCorrection(fresh, 1); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Rollback(0, 2, "reset review"); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.PreviewCorrection("a", sample("b", "extends")); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("historical ID reused: %v", err)
	}
	if got, err := ledger.SnapshotAt(3); err != nil || !reflect.DeepEqual(got.Active, []Assertion{sample("a", "extends"), sample("other", "extends")}) {
		t.Fatalf("rollback projection: %+v %v", got, err)
	}
}

func TestSnapshotsAreDetachedFromLedger(t *testing.T) {
	ledger, err := NewLedger([]Assertion{sample("a", "extends")})
	if err != nil {
		t.Fatal(err)
	}
	p, err := ledger.PreviewCorrection("a", sample("b", "contradicts"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.ConfirmCorrection(p, 0); err != nil {
		t.Fatal(err)
	}
	s := ledger.Snapshot()
	s.Active[0].FromSupport.ChunkID = "modified"
	s.History[0].After.Relation = "modified"
	actual := ledger.Snapshot()
	if actual.Active[0].FromSupport.ChunkID != "chunk-a" || actual.History[0].After.Relation != "contradicts" {
		t.Fatalf("snapshot changed internal state: %+v", actual)
	}
	at, err := ledger.SnapshotAt(1)
	if err != nil || !reflect.DeepEqual(at, actual) {
		t.Fatalf("reducer diverged: %+v != %+v, %v", at, actual, err)
	}
}
