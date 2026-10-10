package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/store"
)

// Read-only practice actions must be served from the database, owner-scoped,
// without calling the worker; only AI work is forwarded.
func TestIntegrationPracticeReadsServedFromStore(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
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
	stamp := time.Now().Format("150405.000000000")
	owner := q(`INSERT INTO users(email,password_hash) VALUES ($1,'synthetic') RETURNING id`, "practice-owner-"+stamp+"@example.invalid")
	other := q(`INSERT INTO users(email,password_hash) VALUES ($1,'synthetic') RETURNING id`, "practice-other-"+stamp+"@example.invalid")
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{owner, other})
	text := "Descent reduces error."
	doc := q(`INSERT INTO documents(user_id,title,status,content_hash) VALUES ($1,'Synthetic','ready','snap') RETURNING id`, owner)
	empty := q(`INSERT INTO documents(user_id,title,status,content_hash) VALUES ($1,'Empty','ready','snap2') RETURNING id`, owner)
	chunk := q(`INSERT INTO chunks(document_id,user_id,chunk_index,content,locator) VALUES ($1,$2,0,$3,'{"page":1}') RETURNING id`, doc, owner, text)
	item := q(`INSERT INTO practice_items(user_id,document_id,chunk_id,document_hash,chunk_hash,payload)
		VALUES ($1,$2,$3,'snap',encode(sha256(convert_to($4::text,'UTF8')),'hex'),
		jsonb_build_object('question','What reduces error?','label','AI-generated practice','version','practice-grounding-v1',
		'answer','Descent','quote','Descent reduces error.','locator','{"page":1}'::jsonb)) RETURNING id`, owner, doc, chunk, text)
	q(`INSERT INTO practice_schedule(item_id,user_id,due_at,last_attempt_at) VALUES ($1,$2,now()-interval '1 hour',now()-interval '2 days') RETURNING item_id`, item, owner)
	for _, a := range []struct {
		phase   string
		exposed bool
		delayed bool
		score   string
	}{{"warmup", false, false, "null"}, {"reading_check", true, false, "0.4"}, {"review", false, true, "0.5"}, {"review", false, true, "1"}} {
		q(`INSERT INTO practice_attempts(user_id,item_id,request_key,request_hash,phase,response,exposed,feedback,delayed_unassisted)
			VALUES ($1,$2,gen_random_uuid(),'h',$3,'r',$4,jsonb_build_object('status','recorded','feedback',jsonb_build_object('score',$5::jsonb)),$6) RETURNING id`,
			owner, item, a.phase, a.exposed, a.score, a.delayed)
	}

	var workerCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workerCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"from-worker","items":[]}`))
	}))
	defer upstream.Close()
	t.Setenv("WORKER_URL", upstream.URL)
	h := New(store.New(pool))
	call := func(user, body string) map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		h.Practice(rec, authed(httptest.NewRequest("POST", "/api/practice", strings.NewReader(body)), user))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
			t.Fatalf("practice response must not be shared-cacheable, got %q", cc)
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	daily := call(owner, `{"action":"daily"}`)
	items := daily["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != item || daily["streak"].(float64) != 1 {
		t.Fatalf("daily = %v", daily)
	}
	first := items[0].(map[string]any)
	if _, leaked := first["answer"]; leaked {
		t.Fatal("daily queue must not expose the reference answer")
	}
	if got := call(other, `{"action":"daily","user_id":"`+owner+`"}`)["items"].([]any); len(got) != 0 {
		t.Fatalf("other user saw owner's items: %v", got)
	}
	progress := call(owner, `{"action":"progress"}`)
	want := map[string]float64{"warmup": 1, "reading_check": 1, "review": 2, "exposed": 1, "delayed_unassisted": 2,
		"delayed_scored": 2, "unscored": 1, "delayed_mean_score": 0.75, "streak": 1, "due": 1}
	for k, v := range want {
		if progress[k] != v {
			t.Errorf("progress[%s] = %v want %v (%v)", k, progress[k], v, progress)
		}
	}
	if progress["estimated_recall"] != nil {
		t.Errorf("no calibrated recall estimate may be reported")
	}
	if got := call(owner, `{"action":"generate","document_id":"`+doc+`"}`)["items"].([]any); len(got) != 1 {
		t.Fatalf("existing items not served from store: %v", got)
	}
	if got := call(other, `{"action":"items","document_id":"`+doc+`"}`)["items"].([]any); len(got) != 0 {
		t.Fatalf("other user saw owner's document items: %v", got)
	}
	if workerCalls.Load() != 0 {
		t.Fatalf("read-only actions called the worker %d times", workerCalls.Load())
	}
	// No live items yet: generation (AI) goes to the worker.
	if got := call(owner, `{"action":"generate","document_id":"`+empty+`"}`); got["status"] != "from-worker" {
		t.Fatalf("first generation must use the worker: %v", got)
	}
	// Re-ingested source: stale items disappear from every read.
	if _, err := pool.Exec(ctx, `UPDATE documents SET content_hash='changed' WHERE id=$1`, doc); err != nil {
		t.Fatal(err)
	}
	if got := call(owner, `{"action":"daily"}`)["items"].([]any); len(got) != 0 {
		t.Fatalf("stale item still due: %v", got)
	}
	if got := call(owner, `{"action":"progress"}`); got["delayed_unassisted"].(float64) != 0 || got["due"].(float64) != 0 {
		t.Fatalf("stale attempts still counted: %v", got)
	}
	if workerCalls.Load() != 1 {
		t.Fatalf("worker calls = %d, want 1", workerCalls.Load())
	}
}
