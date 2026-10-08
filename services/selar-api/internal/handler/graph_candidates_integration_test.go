package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/store"
)

// Issue #117: the Graph drew only learner-reviewed links, so a valid grounded
// candidate awaiting review was invisible and readings looked unconnected.
// Candidates must appear as distinct, non-reviewed "candidate" edges with their
// two exact witnesses; similarity-only passage matches and other owners' links
// must never appear; reviewing the candidate replaces it with the reviewed edge.
func TestIntegrationGraphProjectsGroundedCandidatesDistinctly(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stamp := time.Now().Format("20060102150405.000000000")
	owner := q(`INSERT INTO users(email,password_hash) VALUES ($1,'x') RETURNING id`, "graph-cand-"+stamp+"@example.invalid")
	intruder := q(`INSERT INTO users(email,password_hash) VALUES ($1,'x') RETURNING id`, "graph-cand-x-"+stamp+"@example.invalid")
	defer pool.Exec(ctx, `DELETE FROM users WHERE id=$1 OR id=$2`, owner, intruder)

	concept := "orchard ledger protocol"
	srcText := "The invented orchard ledger protocol records a fabricated harvest."
	tgtText := "A toy vineyard extends the orchard ledger protocol."
	srcDoc := q(`INSERT INTO documents(user_id,title,content_hash,status) VALUES ($1,'Invented notebook','snap-a','ready') RETURNING id`, owner)
	tgtDoc := q(`INSERT INTO documents(user_id,title,content_hash,status) VALUES ($1,'Invented article','snap-b','ready') RETURNING id`, owner)
	srcChunk := q(`INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) VALUES ($1,$2,0,$3,'{"page":1}') RETURNING id`, owner, srcDoc, srcText)
	tgtChunk := q(`INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) VALUES ($1,$2,0,$3,'{"page":4}') RETURNING id`, owner, tgtDoc, tgtText)
	srcModel := q(`INSERT INTO document_mental_models(user_id,document_id,main_claim,key_concepts) VALUES ($1,$2,'claim',ARRAY[$3]) RETURNING id`, owner, srcDoc, concept)
	tgtModel := q(`INSERT INTO document_mental_models(user_id,document_id,main_claim,key_concepts) VALUES ($1,$2,'claim',ARRAY['vineyard rotation']) RETURNING id`, owner, tgtDoc)
	witness := func(doc, chunk, hash, text, quote string, page int) []byte {
		sum := sha256.Sum256([]byte(text))
		out, _ := json.Marshal(map[string]any{"chunk_id": chunk, "asserting_source_id": doc, "source_snapshot_hash": hash,
			"text_sha256": hex.EncodeToString(sum[:]), "quote": quote, "asserted_concept": concept, "locator": map[string]int{"page": page}})
		return out
	}
	link := q(`INSERT INTO mental_model_links(user_id,source_model_id,target_model_id,link_type,similarity,confidence,
		source_evidence_chunk_id,target_evidence_chunk_id,source_evidence,target_evidence,status,created_via)
		VALUES ($1,$2,$3,'concept_overlap',0.8,0.65,$4,$5,$6,$7,'candidate','ai_suggested') RETURNING id`,
		owner, srcModel, tgtModel, srcChunk, tgtChunk,
		witness(srcDoc, srcChunk, "snap-a", srcText, srcText, 1), witness(tgtDoc, tgtChunk, "snap-b", tgtText, tgtText, 4))
	// A similarity-only passage match must never become a graph edge.
	q(`INSERT INTO link_suggestions(user_id,source_chunk_id,target_chunk_id,similarity,relation,status)
		VALUES ($1,$2,$3,0.91,'unclassified','pending') RETURNING id`, owner, srcChunk, tgtChunk)

	h := New(store.New(pool))
	graph := func(user string) (nodes []model.GraphNode, edges []model.GraphEdge) {
		t.Helper()
		w := httptest.NewRecorder()
		h.GetGraph(w, authed(httptest.NewRequest(http.MethodGet, "/api/graph", nil), user))
		if w.Code != http.StatusOK {
			t.Fatalf("graph: %d %s", w.Code, w.Body.String())
		}
		var body struct {
			Nodes []model.GraphNode `json:"nodes"`
			Edges []model.GraphEdge `json:"edges"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Nodes, body.Edges
	}
	crossDocument := func(edges []model.GraphEdge) []model.GraphEdge {
		var out []model.GraphEdge
		for _, e := range edges {
			if strings.HasPrefix(e.Source, "model:") && strings.HasPrefix(e.Target, "model:") {
				out = append(out, e)
			}
		}
		return out
	}

	_, edges := graph(owner)
	cross := crossDocument(edges)
	if len(cross) != 1 {
		t.Fatalf("want exactly one cross-reading candidate edge (no passage-match edge), got %+v", cross)
	}
	e := cross[0]
	if e.ID != "candidate:"+link || e.State != "candidate" || e.CreatedVia != "ai_suggested" || e.CandidateLinkID != link ||
		e.MentalLinkID != "" || e.ReviewRevision != 0 || e.Relation != "concept_overlap" {
		t.Fatalf("candidate must be distinct from a reviewed link: %+v", e)
	}
	if e.Source != "model:"+srcModel || e.Target != "model:"+tgtModel || e.SourceDocumentID != srcDoc || e.TargetDocumentID != tgtDoc ||
		e.SourceQuote != srcText || e.TargetQuote != tgtText || !strings.Contains(e.Explanation, concept) {
		t.Fatalf("candidate must carry both exact witnesses: %+v", e)
	}
	if _, foreign := graph(intruder); len(crossDocument(foreign)) != 0 {
		t.Fatalf("another owner sees the candidate: %+v", foreign)
	}

	// A stale source snapshot hides the candidate (fail closed).
	if _, err := pool.Exec(ctx, `UPDATE documents SET content_hash='refreshed' WHERE id=$1`, tgtDoc); err != nil {
		t.Fatal(err)
	}
	if _, stale := graph(owner); len(crossDocument(stale)) != 0 {
		t.Fatalf("stale candidate still projected: %+v", crossDocument(stale))
	}
	if _, err := pool.Exec(ctx, `UPDATE documents SET content_hash='snap-b' WHERE id=$1`, tgtDoc); err != nil {
		t.Fatal(err)
	}

	// After the learner confirms, the same pair is drawn once, as the reviewed edge.
	zero := int64(0)
	if err := store.New(pool).RespondToMentalModelLink(ctx, owner, link,
		model.MentalModelLinkResponse{Action: model.MentalLinkConfirmed, Revision: &zero}); err != nil {
		t.Fatal(err)
	}
	_, edges = graph(owner)
	cross = crossDocument(edges)
	if len(cross) != 1 || cross[0].ID != "reviewed:"+link || cross[0].State != "confirmed" || cross[0].CandidateLinkID != "" {
		t.Fatalf("confirmed link must replace the candidate edge: %+v", cross)
	}
	// A rejected candidate disappears entirely.
	one := int64(1)
	if err := store.New(pool).RespondToMentalModelLink(ctx, owner, link,
		model.MentalModelLinkResponse{Action: "retracted", Revision: &one, Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, gone := graph(owner); len(crossDocument(gone)) != 0 {
		t.Fatalf("retracted link still drawn: %+v", crossDocument(gone))
	}
}
