package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/selar-dev/selar-api/internal/model"
)

// ── Document model tests ──────────────────────────────────────────────────────

func TestDocStatusConstants(t *testing.T) {
	tests := []struct {
		status   model.DocStatus
		expected string
	}{
		{model.DocStatusUploaded, "uploaded"},
		{model.DocStatusProcessing, "processing"},
		{model.DocStatusReady, "ready"},
		{model.DocStatusFailed, "failed"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("DocStatus: got %q, want %q", tt.status, tt.expected)
		}
	}
}

func TestDocumentJSON(t *testing.T) {
	doc := model.Document{
		ID:        "abc-123",
		UserID:    "user-1",
		Title:     "Attention Is All You Need",
		Authors:   "Vaswani et al.",
		Year:      2017,
		PageCount: 15,
		Status:    model.DocStatusReady,
		AddedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded model.Document
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != doc.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, doc.ID)
	}
	if decoded.Title != doc.Title {
		t.Errorf("Title: got %q, want %q", decoded.Title, doc.Title)
	}
	if decoded.Status != model.DocStatusReady {
		t.Errorf("Status: got %q, want %q", decoded.Status, model.DocStatusReady)
	}
	if decoded.Year != 2017 {
		t.Errorf("Year: got %d, want 2017", decoded.Year)
	}
}

func TestDocumentJSONOmitsInternalFields(t *testing.T) {
	doc := model.Document{
		ID:           "abc",
		FilePath:     "/tmp/secret.pdf",
		GDriveFileID: "drive-xyz",
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	raw := string(data)
	if containsField(raw, "file_path") || containsField(raw, "FilePath") {
		t.Error("FilePath should be omitted from JSON (json:\"-\")")
	}
	if containsField(raw, "gdrive_file_id") || containsField(raw, "GDriveFileID") {
		t.Error("GDriveFileID should be omitted from JSON (json:\"-\")")
	}
}

func TestDocumentStatsDefaults(t *testing.T) {
	stats := model.DocumentStats{}
	if stats.TotalDocuments != 0 || stats.TotalChunks != 0 || stats.ConfirmedLinks != 0 {
		t.Error("Zero-value DocumentStats should have all zeroes")
	}
}

// ── Suggestion model tests ────────────────────────────────────────────────────

func TestSuggestionStatusConstants(t *testing.T) {
	statuses := []model.SuggestionStatus{
		model.SuggestionPending,
		model.SuggestionConfirmed,
		model.SuggestionRejected,
		model.SuggestionRelabeled,
		model.SuggestionExpired,
	}
	expected := []string{"pending", "confirmed", "rejected", "relabeled", "expired"}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("SuggestionStatus[%d]: got %q, want %q", i, s, expected[i])
		}
	}
}

func TestRelationTypeConstants(t *testing.T) {
	relations := []model.RelationType{
		model.RelationRelatedTo,
		model.RelationPrerequisiteOf,
		model.RelationSubConceptOf,
		model.RelationContradicts,
		model.RelationExtends,
	}
	expected := []string{"related_to", "prerequisite_of", "sub_concept_of", "contradicts", "extends"}

	for i, r := range relations {
		if string(r) != expected[i] {
			t.Errorf("RelationType[%d]: got %q, want %q", i, r, expected[i])
		}
	}
}

func TestLinkSuggestionJSON(t *testing.T) {
	label := "my-label"
	s := model.LinkSuggestion{
		ID:         "link-1",
		UserID:     "user-1",
		SrcPage:    3,
		Similarity: 0.85,
		Relation:   model.RelationPrerequisiteOf,
		Status:     model.SuggestionPending,
		UserLabel:  &label,
		Summary:    "Concept A is a prerequisite of Concept B.",
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded model.LinkSuggestion
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Similarity != 0.85 {
		t.Errorf("Similarity: got %f, want 0.85", decoded.Similarity)
	}
	if decoded.SrcPage != 3 {
		t.Errorf("SrcPage: got %d, want 3", decoded.SrcPage)
	}
	if decoded.Relation != model.RelationPrerequisiteOf {
		t.Errorf("Relation: got %q, want %q", decoded.Relation, model.RelationPrerequisiteOf)
	}
	if decoded.Summary != "Concept A is a prerequisite of Concept B." {
		t.Errorf("Summary: got %q", decoded.Summary)
	}
	if decoded.UserLabel == nil || *decoded.UserLabel != "my-label" {
		t.Error("UserLabel not preserved in JSON round-trip")
	}
}

func TestSuggestionResponseJSON(t *testing.T) {
	resp := model.SuggestionResponse{
		Action:          "confirmed",
		Label:           "extends",
		TimeToRespondMs: 1500,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded model.SuggestionResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Action != "confirmed" {
		t.Errorf("Action: got %q, want %q", decoded.Action, "confirmed")
	}
	if decoded.TimeToRespondMs != 1500 {
		t.Errorf("TimeToRespondMs: got %d, want 1500", decoded.TimeToRespondMs)
	}
}

func TestMentalLinkTypeConstants(t *testing.T) {
	relations := []model.MentalLinkType{
		model.MentalLinkConceptOverlap,
		model.MentalLinkClaimExtension,
		model.MentalLinkAssumptionConflict,
		model.MentalLinkQuestionResolution,
	}
	expected := []string{"concept_overlap", "claim_extension", "assumption_conflict", "question_resolution"}
	for index, relation := range relations {
		if string(relation) != expected[index] {
			t.Errorf("MentalLinkType[%d]: got %q, want %q", index, relation, expected[index])
		}
	}
}

func TestDocumentMentalModelJSON(t *testing.T) {
	mentalModel := model.DocumentMentalModel{
		ID:            "model-1",
		DocumentID:    "doc-1",
		MainClaim:     "Retrieval practice improves delayed recall.",
		KeyConcepts:   []string{"retrieval practice", "delayed recall"},
		Assumptions:   []string{"Effortful recall strengthens memory."},
		OpenQuestions: []string{"How long does the effect persist?"},
		Domain:        "learning science",
		Status:        model.MentalModelReady,
	}
	data, err := json.Marshal(mentalModel)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var decoded model.DocumentMentalModel
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.MainClaim != mentalModel.MainClaim || len(decoded.KeyConcepts) != 2 {
		t.Fatalf("Mental model did not survive JSON round-trip: %#v", decoded)
	}
}

// helpers

func containsField(jsonStr, field string) bool {
	return len(jsonStr) > 0 && json.Valid([]byte(jsonStr)) && (len(field) > 0 && (indexOf(jsonStr, `"`+field+`"`) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
