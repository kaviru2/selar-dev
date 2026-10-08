package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/selar-dev/selar-api/internal/model"
	"github.com/selar-dev/selar-api/internal/store"
)

type suggestionGateFixture struct {
	userID string
	docID  string
}

func TestIntegrationSuggestionConditionGate(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("requires TEST_DATABASE_URL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	stamp := time.Now().Format("20060102150405.000000000")
	lockReason := "suggestion-gate-test-" + stamp
	control := seedSuggestionGateFixture(t, pool, "control", "control-"+stamp+"@example.invalid")
	treatment := seedSuggestionGateFixture(t, pool, "treatment_auto", "treatment-"+stamp+"@example.invalid")
	noLock := seedSuggestionGateFixture(t, pool, "treatment_hitl", "no-lock-"+stamp+"@example.invalid")

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM cohort_setting_locks WHERE reason = $1`, lockReason)
		for _, fixture := range []suggestionGateFixture{control, treatment, noLock} {
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, fixture.userID)
		}
		pool.Close()
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO cohort_setting_locks (cohort, setting_key, value, reason)
		VALUES
			('control', 'suggestions.show_on_open', 'false'::jsonb, $1),
			('treatment_auto', 'suggestions.show_on_open', 'true'::jsonb, $1)
		ON CONFLICT (cohort, setting_key) DO UPDATE
		SET value = EXCLUDED.value, reason = EXCLUDED.reason, updated_at = now()`, lockReason); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET preferences = '{"suggestions.show_on_open": false}'::jsonb WHERE id = $1`, noLock.userID); err != nil {
		t.Fatal(err)
	}

	h := New(store.New(pool))
	checkSuggestions := func(t *testing.T, fixture suggestionGateFixture, want int) {
		t.Helper()
		rec := invokeSuggestionList(t, h.ListSuggestions, fixture.userID, fixture.docID, "/api/documents/"+fixture.docID+"/suggestions?page=0")
		if rec.Code != http.StatusOK {
			t.Fatalf("suggestions status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got []model.LinkSuggestion
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode suggestions: %v; body = %s", err, rec.Body.String())
		}
		if want == 0 && strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Fatalf("empty suggestions response shape = %s, want []", rec.Body.String())
		}
		if len(got) != want {
			t.Fatalf("suggestions length = %d, want %d; body = %s", len(got), want, rec.Body.String())
		}
	}
	checkMentalLinks := func(t *testing.T, fixture suggestionGateFixture, want int) {
		t.Helper()
		rec := invokeSuggestionList(t, h.ListMentalModelLinks, fixture.userID, fixture.docID, "/api/mental-model-links?document_id="+fixture.docID)
		if rec.Code != http.StatusOK {
			t.Fatalf("mental-model links status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got []model.MentalModelLink
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode mental-model links: %v; body = %s", err, rec.Body.String())
		}
		if want == 0 && strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Fatalf("empty mental-model links response shape = %s, want []", rec.Body.String())
		}
		if len(got) != want {
			t.Fatalf("mental-model links length = %d, want %d; body = %s", len(got), want, rec.Body.String())
		}
	}

	t.Run("control false lock returns empty lists", func(t *testing.T) {
		checkSuggestions(t, control, 0)
		checkMentalLinks(t, control, 0)
	})
	t.Run("treatment true lock keeps suggestions", func(t *testing.T) {
		checkSuggestions(t, treatment, 1)
		checkMentalLinks(t, treatment, 1)
	})
	t.Run("no lock preserves existing behaviour", func(t *testing.T) {
		checkSuggestions(t, noLock, 1)
		checkMentalLinks(t, noLock, 1)
	})
}

func seedSuggestionGateFixture(t *testing.T, pool *pgxpool.Pool, cohort, email string) suggestionGateFixture {
	t.Helper()
	ctx := context.Background()
	q := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	userID := q(`INSERT INTO users (email, password_hash, cohort) VALUES ($1, 'synthetic', $2) RETURNING id`, email, cohort)
	sourceText := "Spaced retrieval practice improves learning."
	targetText := "Spaced retrieval practice supports durable memory."
	sourceHash := sha256Hex(sourceText)
	targetHash := sha256Hex(targetText)
	sourceDoc := q(`INSERT INTO documents (user_id, title, status, content_hash) VALUES ($1, 'Source', 'ready', $2) RETURNING id`, userID, sourceHash)
	targetDoc := q(`INSERT INTO documents (user_id, title, status, content_hash) VALUES ($1, 'Target', 'ready', $2) RETURNING id`, userID, targetHash)
	sourceChunk := q(`INSERT INTO chunks (document_id, user_id, chunk_index, page_start, content, locator) VALUES ($1, $2, 0, 1, $3, '{"page":1}') RETURNING id`, sourceDoc, userID, sourceText)
	targetChunk := q(`INSERT INTO chunks (document_id, user_id, chunk_index, page_start, content, locator) VALUES ($1, $2, 0, 1, $3, '{"page":1}') RETURNING id`, targetDoc, userID, targetText)
	q(`INSERT INTO link_suggestions (user_id, source_chunk_id, target_chunk_id, similarity, relation, status, summary) VALUES ($1, $2, $3, 0.87, 'unclassified', 'pending', 'Synthetic candidate') RETURNING id`, userID, sourceChunk, targetChunk)
	sourceModel := q(`INSERT INTO document_mental_models (document_id, user_id, main_claim, key_concepts, domain) VALUES ($1, $2, 'Source claim', ARRAY['spaced retrieval practice'], 'learning') RETURNING id`, sourceDoc, userID)
	targetModel := q(`INSERT INTO document_mental_models (document_id, user_id, main_claim, key_concepts, domain) VALUES ($1, $2, 'Target claim', ARRAY['spaced retrieval practice'], 'learning') RETURNING id`, targetDoc, userID)
	sourceEvidence := evidenceJSON(sourceChunk, sourceDoc, sourceHash, sourceText)
	targetEvidence := evidenceJSON(targetChunk, targetDoc, targetHash, targetText)
	q(`INSERT INTO mental_model_links (
		user_id, source_model_id, target_model_id, link_type, similarity, confidence,
		bridge_explanation, source_evidence_chunk_id, target_evidence_chunk_id,
		source_evidence, target_evidence, status, created_via
	) VALUES ($1, $2, $3, 'concept_overlap', 0.91, 0.88, 'Synthetic candidate', $4, $5, $6::jsonb, $7::jsonb, 'candidate', 'ai_suggested') RETURNING id`,
		userID, sourceModel, targetModel, sourceChunk, targetChunk, sourceEvidence, targetEvidence)
	return suggestionGateFixture{userID: userID, docID: sourceDoc}
}

func invokeSuggestionList(t *testing.T, handler http.HandlerFunc, userID, docID, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = authed(req, userID)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", docID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func evidenceJSON(chunkID, documentID, snapshotHash, text string) string {
	payload := map[string]any{
		"chunk_id":             chunkID,
		"asserting_source_id":  documentID,
		"source_snapshot_hash": snapshotHash,
		"text_sha256":          sha256Hex(text),
		"quote":                text,
		"asserted_concept":     "spaced retrieval practice",
		"locator":              map[string]any{"page": 1},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}
