package store

import (
	"encoding/json"
	"github.com/selar-dev/selar-api/internal/model"
	"testing"
)

func TestAnnotationEditDeleteOwnershipAndSource(t *testing.T) {
	s, ctx, owner, intruder, _, doc, cleanup := reviewFixture(t)
	defer cleanup()
	a := model.Annotation{UserID: owner, DocumentID: doc, Page: 1, BBox: json.RawMessage(`[]`), Color: model.ColorYellow, Type: model.AnnotationHighlight, Anchor: &model.TextAnchor{Version: 1, SourceHash: "snapshot-old", Exact: "quote", Start: 0, End: 5}}
	if err := s.CreateAnnotation(ctx, &a); err != nil {
		t.Fatal(err)
	}
	color := model.ColorCoral
	comment := "Reflection"
	hash := "snapshot-old"
	patch := model.AnnotationPatch{Color: &color, Comment: &comment, SourceHash: hash}
	if _, err := s.UpdateAnnotation(ctx, a.ID, intruder, patch); err == nil {
		t.Fatal("foreign owner edited annotation")
	}
	if err := s.DeleteAnnotation(ctx, a.ID, intruder); err == nil {
		t.Fatal("foreign delete must return not found")
	}
	edited, err := s.UpdateAnnotation(ctx, a.ID, owner, patch)
	if err != nil || edited.Color != color || edited.Comment != comment || edited.Anchor.Exact != "quote" {
		t.Fatalf("edit: %+v %v", edited, err)
	}
	_, err = s.pool.Exec(ctx, `UPDATE documents SET content_hash='changed' WHERE id=$1`, doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateAnnotation(ctx, a.ID, owner, patch); err != ErrAnnotationSourceChanged {
		t.Fatalf("source edit: %v", err)
	}
	if err = s.DeleteAnnotation(ctx, a.ID, owner); err != ErrAnnotationSourceChanged {
		t.Fatalf("source delete: %v", err)
	}
	anns, err := s.ListAnnotations(ctx, owner, doc, 0)
	if err != nil || len(anns) != 1 || anns[0].Comment != comment {
		t.Fatalf("mismatch mutated: %+v %v", anns, err)
	}
	_, _ = s.pool.Exec(ctx, `UPDATE documents SET content_hash='snapshot-old' WHERE id=$1`, doc)
	if err = s.DeleteAnnotation(ctx, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	anns, err = s.ListAnnotations(ctx, owner, doc, 0)
	if err != nil || len(anns) != 0 {
		t.Fatalf("delete not persisted: %v", err)
	}
}

func TestAnnotationLegacyRectangleEdit(t *testing.T) {
	s, ctx, owner, _, _, doc, cleanup := reviewFixture(t)
	defer cleanup()
	a := model.Annotation{UserID: owner, DocumentID: doc, Page: 1, BBox: json.RawMessage(`[{"x":0.1,"y":0.2,"w":0.3,"h":0.04}]`), Color: model.ColorYellow, Type: model.AnnotationHighlight}
	if err := s.CreateAnnotation(ctx, &a); err != nil {
		t.Fatal(err)
	}
	comment := "legacy note"
	edited, err := s.UpdateAnnotation(ctx, a.ID, owner, model.AnnotationPatch{Comment: &comment})
	if err != nil || edited.Anchor != nil || edited.Comment != comment || string(edited.BBox) == "null" {
		t.Fatalf("legacy changed: %+v %v", edited, err)
	}
}

func TestAnnotationRejectsSourceMismatch(t *testing.T) {
	s, ctx, owner, _, _, doc, cleanup := reviewFixture(t)
	defer cleanup()
	a := model.Annotation{UserID: owner, DocumentID: doc, Page: 1, BBox: json.RawMessage(`[]`), Color: model.ColorYellow, Type: model.AnnotationHighlight, Anchor: &model.TextAnchor{Version: 1, SourceHash: "different", Exact: "quote", Start: 0, End: 5}}
	if err := s.CreateAnnotation(ctx, &a); err == nil {
		t.Fatal("source mismatch accepted")
	}
	var count int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM annotations WHERE user_id=$1`, owner).Scan(&count)
	if count != 0 {
		t.Fatal("mismatch mutated annotations")
	}
}

func TestAnnotationPersistsTextAnchor(t *testing.T) {
	s, ctx, owner, _, _, doc, cleanup := reviewFixture(t)
	defer cleanup()
	var a model.Annotation
	raw := `{"page":1,"bbox":[{"x":0.1,"y":0.2,"w":0.3,"h":0.04}],"color":"yellow","type":"highlight","anchor":{"version":1,"source_hash":"snapshot-old","exact":"Gradient descent","prefix":"","suffix":" optimization","start":0,"end":16}}`
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatal(err)
	}
	a.UserID = owner
	a.DocumentID = doc
	if err := s.CreateAnnotation(ctx, &a); err != nil {
		t.Fatal(err)
	}
	anns, err := s.ListAnnotations(ctx, owner, doc, 0)
	if err != nil || len(anns) != 1 {
		t.Fatalf("list %v %v", anns, err)
	}
	data, _ := json.Marshal(anns[0])
	var saved map[string]any
	_ = json.Unmarshal(data, &saved)
	if saved["anchor"] == nil {
		t.Fatal("text anchor was discarded")
	}
}

func TestAnnotationRejectsForeignChunk(t *testing.T) {
	s, ctx, owner, intruder, _, doc, cleanup := reviewFixture(t)
	defer cleanup()
	var foreignDoc, foreignChunk string
	if err := s.pool.QueryRow(ctx, `INSERT INTO documents(user_id,title) VALUES ($1,'foreign') RETURNING id`, intruder).Scan(&foreignDoc); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `INSERT INTO chunks(user_id,document_id,chunk_index,content) VALUES ($1,$2,0,'private') RETURNING id`, intruder, foreignDoc).Scan(&foreignChunk); err != nil {
		t.Fatal(err)
	}
	a := model.Annotation{UserID: owner, DocumentID: doc, ChunkID: &foreignChunk, Page: 1, BBox: json.RawMessage(`[{"x":0.1,"y":0.1,"w":0.2,"h":0.03}]`), Color: model.ColorYellow, Type: model.AnnotationHighlight}
	if err := s.CreateAnnotation(ctx, &a); err == nil {
		t.Fatal("foreign chunk accepted")
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM annotations WHERE user_id=$1`, owner).Scan(&count); err != nil || count != 0 {
		t.Fatalf("mutation after rejection: %d %v", count, err)
	}
}
