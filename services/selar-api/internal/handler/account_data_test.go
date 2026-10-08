package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/selar-dev/selar-api/internal/storage"
	"github.com/selar-dev/selar-api/internal/storage/storagetest"
)

func TestDeleteAccountRequiresConfirmationBeforePersistence(t *testing.T) {
	h := New(nil)
	const user = "00000000-0000-0000-0000-000000000001"
	for _, body := range []string{`{"current_password":"x"}`, `{"confirm":"delete my account"}`, `{"current_password":"x","confirm":"yes"}`, `nope`} {
		if rec := sendJSON(t, h.DeleteAccount, http.MethodPost, "/", body, user); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", body, rec.Code)
		}
	}
}

// seedAccountData gives the fixture user a PDF document with a chunk, an
// annotation, a reviewed suggestion and a stored object in fake S3.
func seedAccountData(t *testing.T, f *accountFixture, fake *storagetest.FakeS3) (docID, uploadKey string) {
	t.Helper()
	ctx := context.Background()
	q := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if err := f.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	docID = q(`INSERT INTO documents(user_id,title,authors,content_hash,status,source_type,page_count)
		VALUES ($1,'Synthetic Reading','Invented Author','h1','ready','pdf',1) RETURNING id`, f.id)
	other := q(`INSERT INTO documents(user_id,title,content_hash,status) VALUES ($1,'Second','h2','ready') RETURNING id`, f.id)
	c1 := q(`INSERT INTO chunks(user_id,document_id,chunk_index,content) VALUES ($1,$2,0,'alpha') RETURNING id`, f.id, docID)
	c2 := q(`INSERT INTO chunks(user_id,document_id,chunk_index,content) VALUES ($1,$2,0,'beta') RETURNING id`, f.id, other)
	q(`INSERT INTO link_suggestions(user_id,source_chunk_id,target_chunk_id,similarity,status,user_label)
		VALUES ($1,$2,$3,0.8,'confirmed','synthetic label') RETURNING id`, f.id, c1, c2)
	q(`INSERT INTO annotations(user_id,document_id,page,bbox,color,type,comment)
		VALUES ($1,$2,1,'[]','yellow','note','invented note') RETURNING id`, f.id, docID)
	uploadKey = "users/" + f.id + "/uploads/55555555-5555-5555-5555-555555555555.pdf"
	fake.Seed(uploadKey, []byte("%PDF-1.7 synthetic"), "application/pdf")
	fake.Seed(docID+"/assets/figure-1.png", []byte("png"), "image/png")
	source := q(`INSERT INTO content_sources(user_id,kind,title) VALUES ($1,'pdf','Synthetic Reading') RETURNING id`, f.id)
	run := q(`INSERT INTO ingestion_runs(source_id,document_id,user_id) VALUES ($1,$2,$3) RETURNING id`, source, docID, f.id)
	q(`INSERT INTO ingestion_jobs(run_id,source_id,document_id,user_id,source_type,file_path,title,status)
		VALUES ($1,$2,$3,$4,'pdf',$5,'Synthetic Reading','queued') RETURNING id`, run, source, docID, f.id, "s3://selar-test/"+uploadKey)
	return docID, uploadKey
}

func withFakeS3(t *testing.T, f *accountFixture) *storagetest.FakeS3 {
	t.Helper()
	fake, server := storagetest.NewFakeS3WithState(t, "selar-test")
	objects, err := storage.NewS3(storage.S3Config{Endpoint: server.URL, Bucket: "selar-test", Region: "auto",
		AccessKeyID: "test-access", SecretAccessKey: "test-secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	f.h.SetStorage(objects)
	return fake
}

func TestIntegrationExportMyDataIsAZipOfOwnRowsOnly(t *testing.T) {
	f := newAccountFixture(t)
	fake := withFakeS3(t, f)
	docID, _ := seedAccountData(t, f, fake)
	sendJSON(t, f.h.UpdateProfile, http.MethodPatch, "/", `{"display_name":"Synthetic Tester"}`, f.id)

	rec := sendJSON(t, f.h.ExportMyData, http.MethodGet, "/", "", f.id)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content type %q", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("export must download as an attachment")
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, file := range zr.File {
		rc, _ := file.Open()
		files[file.Name], _ = io.ReadAll(rc)
		rc.Close()
	}
	for _, name := range []string{"README.txt", "account.json", "documents.json", "annotations.json", "link_decisions.json", "mental_model_links.json", "quiz_attempts.json", "reading_sessions.json"} {
		if _, ok := files[name]; !ok {
			t.Fatalf("export is missing %s (has %v)", name, len(files))
		}
	}
	var account map[string]any
	_ = json.Unmarshal(files["account.json"], &account)
	if account["email"] != f.email || account["display_name"] != "Synthetic Tester" {
		t.Fatalf("account.json = %s", files["account.json"])
	}
	all := string(bytes.Join([][]byte{files["account.json"], files["documents.json"], files["annotations.json"], files["link_decisions.json"]}, nil))
	if strings.Contains(all, "password") || strings.Contains(all, "session_version") {
		t.Fatal("export must not contain credentials")
	}
	var docs []map[string]any
	_ = json.Unmarshal(files["documents.json"], &docs)
	if len(docs) != 2 || !strings.Contains(string(files["documents.json"]), docID) {
		t.Fatalf("documents.json = %s", files["documents.json"])
	}
	var decisions []map[string]any
	_ = json.Unmarshal(files["link_decisions.json"], &decisions)
	if len(decisions) != 1 || decisions[0]["status"] != "confirmed" || decisions[0]["user_label"] != "synthetic label" {
		t.Fatalf("link_decisions.json = %s", files["link_decisions.json"])
	}
	if !strings.Contains(string(files["annotations.json"]), "invented note") {
		t.Fatalf("annotations.json = %s", files["annotations.json"])
	}

	// Another user's export never includes this user's rows.
	stranger := newAccountFixture(t)
	other := sendJSON(t, f.h.ExportMyData, http.MethodGet, "/", "", stranger.id)
	if bytes.Contains(other.Body.Bytes(), []byte(docID)) {
		t.Fatal("export leaked another user's document")
	}
}

func TestIntegrationDeleteAccountRemovesRowsAndStoredObjects(t *testing.T) {
	f := newAccountFixture(t)
	fake := withFakeS3(t, f)
	docID, uploadKey := seedAccountData(t, f, fake)
	strangerKey := "users/00000000-0000-0000-0000-00000000beef/uploads/66666666-6666-6666-6666-666666666666.pdf"
	fake.Seed(strangerKey, []byte("%PDF other"), "application/pdf")
	orphan := "users/" + f.id + "/uploads/77777777-7777-7777-7777-777777777777.pdf"
	fake.Seed(orphan, []byte("%PDF never completed"), "application/pdf")

	if rec := sendJSON(t, f.h.DeleteAccount, http.MethodPost, "/", `{"current_password":"wrong password 9","confirm":"delete my account"}`, f.id); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong password accepted: %d", rec.Code)
	}
	rec := sendJSON(t, f.h.DeleteAccount, http.MethodPost, "/", `{"current_password":"`+fixturePassword+`","confirm":"delete my account"}`, f.id)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	var users, docs, chunks, suggestions, annotations int
	if err := f.pool.QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM users WHERE id=$1), (SELECT count(*) FROM documents WHERE user_id=$1),
		(SELECT count(*) FROM chunks WHERE user_id=$1), (SELECT count(*) FROM link_suggestions WHERE user_id=$1),
		(SELECT count(*) FROM annotations WHERE user_id=$1)`, f.id).Scan(&users, &docs, &chunks, &suggestions, &annotations); err != nil {
		t.Fatal(err)
	}
	if users+docs+chunks+suggestions+annotations != 0 {
		t.Fatalf("rows survived: users=%d docs=%d chunks=%d suggestions=%d annotations=%d", users, docs, chunks, suggestions, annotations)
	}
	if fake.Has(uploadKey) || fake.Has(docID+"/assets/figure-1.png") || fake.Has(orphan) {
		t.Fatalf("stored objects survived: %v", fake.Keys())
	}
	if !fake.Has(strangerKey) {
		t.Fatal("another user's object was deleted")
	}
}

func TestIntegrationDeleteAccountWithReviewedGroundedLink(t *testing.T) {
	f := newAccountFixture(t)
	ctx := context.Background()
	q := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if err := f.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// A grounded mental-model link with a review event: the cascade must not
	// trip the evidence triggers (the lesson from #81).
	src := q(`INSERT INTO documents(user_id,title,content_hash,status) VALUES ($1,'a','snap-a','ready') RETURNING id`, f.id)
	tgt := q(`INSERT INTO documents(user_id,title,content_hash,status) VALUES ($1,'b','snap-b','ready') RETURNING id`, f.id)
	srcText, tgtText := "Spaced retrieval practice is discussed.", "Spaced retrieval practice is compared."
	sc := q(`INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) VALUES ($1,$2,0,$3,'{"page":1}') RETURNING id`, f.id, src, srcText)
	tc := q(`INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) VALUES ($1,$2,0,$3,'{"page":1}') RETURNING id`, f.id, tgt, tgtText)
	sm := q(`INSERT INTO document_mental_models(user_id,document_id,main_claim,key_concepts) VALUES ($1,$2,'c',ARRAY['spaced retrieval practice']) RETURNING id`, f.id, src)
	tm := q(`INSERT INTO document_mental_models(user_id,document_id,main_claim,key_concepts) VALUES ($1,$2,'c',ARRAY['spaced retrieval practice']) RETURNING id`, f.id, tgt)
	witness := func(doc, chunk, hash, text string) string {
		b, _ := json.Marshal(map[string]any{"chunk_id": chunk, "asserting_source_id": doc, "source_snapshot_hash": hash,
			"text_sha256": sha256Hex(text), "quote": "Spaced retrieval practice", "asserted_concept": "spaced retrieval practice", "locator": map[string]int{"page": 1}})
		return string(b)
	}
	link := q(`INSERT INTO mental_model_links(user_id,source_model_id,target_model_id,link_type,source_evidence_chunk_id,target_evidence_chunk_id,source_evidence,target_evidence)
		VALUES ($1,$2,$3,'concept_overlap',$4,$5,$6,$7) RETURNING id`, f.id, sm, tm, sc, tc, witness(src, sc, "snap-a", srcText), witness(tgt, tc, "snap-b", tgtText))
	q(`INSERT INTO mental_link_review_events(user_id,link_id,revision,action,before_status,after_status) VALUES ($1,$2,1,'confirmed','suggested','confirmed') RETURNING id`, f.id, link)

	rec := sendJSON(t, f.h.DeleteAccount, http.MethodPost, "/", `{"current_password":"`+fixturePassword+`","confirm":"delete my account"}`, f.id)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete with reviewed link: %d %s", rec.Code, rec.Body.String())
	}
	var left int
	_ = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users WHERE id=$1)+(SELECT count(*) FROM mental_model_links WHERE user_id=$1)+(SELECT count(*) FROM mental_link_review_events WHERE user_id=$1)`, f.id).Scan(&left)
	if left != 0 {
		t.Fatalf("%d rows survived account deletion", left)
	}
}
