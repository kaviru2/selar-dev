package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// exportQueries are the per-user tables included in "Export my data". Each
// query returns one JSON array (never NULL) of the user's own rows. Raw
// passage text, embeddings and credentials are excluded: the export is the
// learner's metadata, decisions and activity, not a copy of the readings.
var exportQueries = []struct {
	File  string
	Query string
}{
	{"documents.json", `SELECT id, title, authors, year, page_count, status, source_type, source_url,
		canonical_url, mime_type, added_at, processed_at FROM documents WHERE user_id = $1 ORDER BY added_at`},
	{"annotations.json", `SELECT id, document_id, page, bbox, color, type, comment, created_at, updated_at
		FROM annotations WHERE user_id = $1 ORDER BY created_at`},
	{"link_decisions.json", `SELECT s.id, sc.document_id AS source_document_id, tc.document_id AS target_document_id,
		s.similarity, s.relation, s.status, s.user_label, s.time_to_respond_ms, s.suggested_at, s.responded_at
		FROM link_suggestions s
		JOIN chunks sc ON sc.id = s.source_chunk_id
		JOIN chunks tc ON tc.id = s.target_chunk_id
		WHERE s.user_id = $1 ORDER BY s.suggested_at`},
	{"mental_model_links.json", `SELECT l.id, l.link_type, l.status, l.user_label, l.created_via, l.review_revision,
		l.suggested_at, l.responded_at,
		COALESCE((SELECT json_agg(json_build_object('revision', e.revision, 'action', e.action,
			'before_status', e.before_status, 'after_status', e.after_status, 'after_label', e.after_label,
			'reason', e.reason, 'occurred_at', e.occurred_at) ORDER BY e.revision)
			FROM mental_link_review_events e WHERE e.link_id = l.id), '[]') AS review_events
		FROM mental_model_links l WHERE l.user_id = $1 ORDER BY l.suggested_at`},
	{"quiz_attempts.json", `SELECT a.id, a.quiz_id, q.title AS quiz_title, q.kind AS quiz_kind, a.attempt_number,
		a.started_at, a.submitted_at, a.submit_reason, a.max_points, a.score,
		COALESCE((SELECT json_agg(json_build_object('question_id', x.question_id, 'selected', x.selected,
			'answer_text', x.answer_text, 'answered_at', x.answered_at, 'time_on_question_ms', x.time_on_question_ms,
			'is_correct', x.is_correct, 'manual_score', x.manual_score, 'grader_note', x.grader_note))
			FROM quiz_answers x WHERE x.attempt_id = a.id), '[]') AS answers
		FROM quiz_attempts a JOIN quizzes q ON q.id = a.quiz_id WHERE a.user_id = $1 ORDER BY a.started_at`},
	{"email_notices.json", `SELECT id, kind, quiz_id, window_start, window_end, transport, subject, status, created_at
		FROM notification_log WHERE user_id = $1 ORDER BY created_at`},
	{"reading_sessions.json", `SELECT id, document_id, started_at, ended_at, pages_viewed, max_scroll_depth
		FROM reading_sessions WHERE user_id = $1 ORDER BY started_at`},
	{"chat_threads.json", `SELECT t.id, t.title, t.created_at,
		COALESCE((SELECT json_agg(json_build_object('role', m.role, 'content', m.content, 'created_at', m.created_at)
			ORDER BY m.created_at) FROM chat_messages m WHERE m.thread_id = t.id), '[]') AS messages
		FROM chat_threads t WHERE t.user_id = $1 AND t.deleted_at IS NULL ORDER BY t.created_at`},
	{"learning_events.json", `SELECT event_type, occurred_at, document_id, suggestion_id, mental_link_id, payload, source
		FROM learning_events WHERE user_id = $1 ORDER BY occurred_at`},
	{"analytics_events.json", `SELECT event, props, source, occurred_at
		FROM analytics_events WHERE user_id = $1 ORDER BY occurred_at`},
}

// ExportUserData returns the user's account summary and one JSON document
// per exported table, keyed by file name.
func (s *Store) ExportUserData(ctx context.Context, userID string) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	var account json.RawMessage
	if err := s.pool.QueryRow(ctx, `SELECT json_build_object('id', id, 'email', email, 'display_name', display_name,
		'cohort', cohort, 'preferences', preferences, 'created_at', created_at,
		'research_consented_at', consented_at, 'research_consent_version', consent_version) FROM users WHERE id = $1`, userID).Scan(&account); err != nil {
		return nil, err
	}
	out["account.json"] = account
	for _, q := range exportQueries {
		var rows json.RawMessage
		if err := s.pool.QueryRow(ctx, `SELECT COALESCE(json_agg(row_to_json(t)), '[]') FROM (`+q.Query+`) t`, userID).Scan(&rows); err != nil {
			return nil, fmt.Errorf("export %s: %w", q.File, err)
		}
		out[q.File] = rows
	}
	return out, nil
}

// StoredDocument identifies a document's uploaded PDF so storage can be
// cleaned after the rows are gone.
type StoredDocument struct {
	ID         string
	PDFLocator string
}

// ListStoredDocuments returns every document of the user with the locator of
// its latest PDF ingestion job (empty for web/text sources).
func (s *Store) ListStoredDocuments(ctx context.Context, userID string) ([]StoredDocument, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.id::text, COALESCE((SELECT j.file_path FROM ingestion_jobs j
		WHERE j.document_id = d.id AND j.source_type = 'pdf' ORDER BY j.created_at DESC LIMIT 1), '')
		FROM documents d WHERE d.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []StoredDocument
	for rows.Next() {
		var d StoredDocument
		if err := rows.Scan(&d.ID, &d.PDFLocator); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// DeleteUser removes the account and all of its rows in one transaction.
// Grounded mental-model links are deleted first, as in DeleteDocument (#81):
// it keeps the cascade from ever rewriting their evidence pointers through
// ON DELETE SET NULL. Since migration 012 the triggers tolerate that rewrite,
// so this is defence in depth rather than required for correctness.
func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM mental_model_links WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete links: %w", err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return tx.Commit(ctx)
}
