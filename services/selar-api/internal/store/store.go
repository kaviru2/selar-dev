// Package store provides the Postgres data-access layer for SELAR.
// All queries use pgx directly (no ORM) against the schema defined
// in migrations/001_init.sql — users, documents, chunks, link_suggestions,
// concepts, concept_edges, annotations, reading_sessions, quizzes.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/model"
)

var ErrMentalModelLinkNotFound = errors.New("mental-model link not found")

// Store wraps the database connection pool and provides data access methods.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store with the given connection pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ============================================================
// Users
// ============================================================

func (s *Store) CreateUser(ctx context.Context, email, passwordHash string, cohort model.Cohort) (*model.User, error) {
	u := &model.User{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, cohort)
		 VALUES ($1, $2, $3)
		 RETURNING id, email, cohort, drive_connected, preferences, created_at`,
		email, passwordHash, cohort,
	).Scan(&u.ID, &u.Email, &u.Cohort, &u.DriveConnected, &u.Preferences, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	u := &model.User{}
	var prefsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, cohort, drive_connected, preferences, created_at
		 FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.Email, &u.Password, &u.Cohort, &u.DriveConnected, &prefsJSON, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if prefsJSON != nil {
		_ = json.Unmarshal(prefsJSON, &u.Preferences)
	}
	return u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	u := &model.User{}
	var prefsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, cohort, drive_connected, preferences, created_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Email, &u.Cohort, &u.DriveConnected, &prefsJSON, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if prefsJSON != nil {
		_ = json.Unmarshal(prefsJSON, &u.Preferences)
	}
	return u, nil
}

func (s *Store) UpdateUserPreferences(ctx context.Context, id string, prefs map[string]any) error {
	prefsJSON, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE users SET preferences = $1 WHERE id = $2`, prefsJSON, id)
	return err
}

// ============================================================
// Documents
// ============================================================

func (s *Store) ListDocuments(ctx context.Context, userID string) ([]model.Document, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT d.id, d.user_id, d.title, d.authors, d.year, d.page_count, d.status, d.progress,
		        d.source_id, d.source_type, d.source_url, d.canonical_url, d.content_hash,
		        d.mime_type, d.metadata, d.fetched_at, d.visible,
		        COALESCE(job.status, ''), COALESCE(job.error, ''), added_at, processed_at
		 FROM documents d
		 LEFT JOIN LATERAL (
		     SELECT status, error FROM ingestion_jobs j WHERE j.document_id = d.id
		     ORDER BY created_at DESC LIMIT 1
		 ) job ON true
		 WHERE d.user_id = $1 AND d.visible ORDER BY added_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []model.Document
	for rows.Next() {
		var d model.Document
		if err := rows.Scan(&d.ID, &d.UserID, &d.Title, &d.Authors, &d.Year, &d.PageCount, &d.Status, &d.Progress,
			&d.SourceID, &d.SourceType, &d.SourceURL, &d.CanonicalURL, &d.ContentHash,
			&d.MimeType, &d.Metadata, &d.FetchedAt, &d.Visible, &d.IngestionStatus,
			&d.IngestionError, &d.AddedAt, &d.ProcessedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func (s *Store) GetDocument(ctx context.Context, id, userID string) (*model.Document, error) {
	d := &model.Document{}
	err := s.pool.QueryRow(ctx,
		`SELECT d.id, d.user_id, d.title, d.authors, d.year, d.page_count, d.status, d.progress,
		        d.source_id, d.source_type, d.source_url, d.canonical_url, d.content_hash,
		        d.mime_type, d.metadata, d.fetched_at, d.visible,
		        COALESCE(job.status, ''), COALESCE(job.error, ''), d.added_at, d.processed_at
		 FROM documents d
		 LEFT JOIN LATERAL (
		     SELECT status, error FROM ingestion_jobs j WHERE j.document_id = d.id
		     ORDER BY created_at DESC LIMIT 1
		 ) job ON true
		 WHERE d.id = $1 AND d.user_id = $2`, id, userID,
	).Scan(&d.ID, &d.UserID, &d.Title, &d.Authors, &d.Year, &d.PageCount, &d.Status, &d.Progress,
		&d.SourceID, &d.SourceType, &d.SourceURL, &d.CanonicalURL, &d.ContentHash,
		&d.MimeType, &d.Metadata, &d.FetchedAt, &d.Visible, &d.IngestionStatus,
		&d.IngestionError, &d.AddedAt, &d.ProcessedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Store) CreateDocument(ctx context.Context, d *model.Document) error {
	if d.SourceType == "" {
		d.SourceType = "pdf"
	}
	if len(d.Metadata) == 0 {
		d.Metadata = json.RawMessage(`{}`)
	}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO documents (
		    user_id, title, authors, year, page_count, status, progress, file_path,
		    gdrive_file_id, source_id, source_type, source_url, canonical_url,
		    content_hash, mime_type, metadata, fetched_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING id, added_at`,
		d.UserID, d.Title, d.Authors, d.Year, d.PageCount, d.Status, d.Progress, d.FilePath,
		d.GDriveFileID, d.SourceID, d.SourceType, d.SourceURL, d.CanonicalURL,
		d.ContentHash, d.MimeType, d.Metadata, d.FetchedAt,
	).Scan(&d.ID, &d.AddedAt)
	if err == nil {
		d.Visible = true
	}
	return err
}

func (s *Store) GetDocumentContent(ctx context.Context, id, userID string) (*model.DocumentContent, error) {
	doc, err := s.GetDocument(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	blockRows, err := s.pool.Query(ctx, `
		SELECT id, document_id, block_index, kind, text, locator, metadata
		FROM content_blocks WHERE document_id = $1 AND user_id = $2
		ORDER BY block_index`, id, userID)
	if err != nil {
		return nil, err
	}
	defer blockRows.Close()
	blocks := []model.ContentBlock{}
	for blockRows.Next() {
		var block model.ContentBlock
		if err := blockRows.Scan(&block.ID, &block.DocumentID, &block.BlockIndex, &block.Kind,
			&block.Text, &block.Locator, &block.Metadata); err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	if err := blockRows.Err(); err != nil {
		return nil, err
	}

	assetRows, err := s.pool.Query(ctx, `
		SELECT id, document_id, block_index, kind, source_url, mime_type, width, height,
		       content_hash, caption, alt_text, description, locator, embedding_model, embedding_version
		FROM assets WHERE document_id = $1 AND user_id = $2
		ORDER BY block_index NULLS LAST, created_at`, id, userID)
	if err != nil {
		return nil, err
	}
	defer assetRows.Close()
	assets := []model.Asset{}
	for assetRows.Next() {
		var asset model.Asset
		if err := assetRows.Scan(&asset.ID, &asset.DocumentID, &asset.BlockIndex, &asset.Kind,
			&asset.SourceURL, &asset.MimeType, &asset.Width, &asset.Height, &asset.ContentHash,
			&asset.Caption, &asset.AltText, &asset.Description, &asset.Locator,
			&asset.EmbeddingModel, &asset.EmbeddingVersion); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	if err := assetRows.Err(); err != nil {
		return nil, err
	}
	return &model.DocumentContent{Document: doc, Blocks: blocks, Assets: assets}, nil
}

func (s *Store) GetAsset(ctx context.Context, documentID, assetID, userID string) (*model.Asset, error) {
	asset := &model.Asset{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, document_id, block_index, kind, storage_path, source_url, mime_type,
		       width, height, content_hash, caption, alt_text, description, locator,
		       embedding_model, embedding_version
		FROM assets WHERE id = $1 AND document_id = $2 AND user_id = $3`,
		assetID, documentID, userID,
	).Scan(&asset.ID, &asset.DocumentID, &asset.BlockIndex, &asset.Kind, &asset.StoragePath,
		&asset.SourceURL, &asset.MimeType, &asset.Width, &asset.Height, &asset.ContentHash,
		&asset.Caption, &asset.AltText, &asset.Description, &asset.Locator,
		&asset.EmbeddingModel, &asset.EmbeddingVersion)
	if err != nil {
		return nil, err
	}
	return asset, nil
}

func (s *Store) DeleteDocument(ctx context.Context, id, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM documents WHERE id = $1 AND user_id = $2`, id, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx,
		`DELETE FROM concepts c WHERE c.user_id = $1
		 AND NOT EXISTS (SELECT 1 FROM chunk_concepts cc WHERE cc.concept_id = c.id)`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetDocumentStats(ctx context.Context, userID string) (*model.DocumentStats, error) {
	stats := &model.DocumentStats{}
	err := s.pool.QueryRow(ctx,
		`SELECT
		   COUNT(DISTINCT d.id),
		   COALESCE(SUM((SELECT COUNT(*) FROM chunks c WHERE c.document_id = d.id)), 0),
		   COALESCE((SELECT COUNT(*) FROM link_suggestions ls
		     WHERE ls.user_id = $1 AND ls.status = 'confirmed'), 0)
		 FROM documents d WHERE d.user_id = $1 AND d.visible`,
		userID,
	).Scan(&stats.TotalDocuments, &stats.TotalChunks, &stats.ConfirmedLinks)
	return stats, err
}

// ============================================================
// Link Suggestions
// ============================================================

func (s *Store) ListSuggestions(ctx context.Context, userID, docID string, page int) ([]model.LinkSuggestion, error) {
	rows, err := s.pool.Query(ctx,
		`WITH oriented AS (
		   SELECT ls.id, ls.user_id,
		          CASE WHEN sc.document_id = $2 THEN sc.id ELSE tc.id END AS source_chunk_id,
		          CASE WHEN sc.document_id = $2 THEN tc.id ELSE sc.id END AS target_chunk_id,
		          ls.similarity, 'unclassified' AS relation, ls.status, ls.user_label,
		          ls.time_to_respond_ms, ls.suggested_at, ls.responded_at,
		          CASE WHEN sc.document_id = $2 THEN sc.content ELSE tc.content END AS src_text,
		          CASE WHEN sc.document_id = $2 THEN tc.content ELSE sc.content END AS tgt_text,
		          CASE WHEN sc.document_id = $2 THEN sd.id ELSE td.id END AS src_document_id,
		          CASE WHEN sc.document_id = $2 THEN td.id ELSE sd.id END AS tgt_document_id,
		          CASE WHEN sc.document_id = $2 THEN sd.title ELSE td.title END AS src_doc,
		          CASE WHEN sc.document_id = $2 THEN td.title ELSE sd.title END AS tgt_doc,
		          CASE WHEN sc.document_id = $2 THEN sc.page_start ELSE tc.page_start END AS src_page,
		          CASE WHEN sc.document_id = $2 THEN tc.page_start ELSE sc.page_start END AS tgt_page,
		          ls.summary,
		          CASE WHEN sc.document_id = $2 THEN sc.bboxes ELSE tc.bboxes END AS src_bboxes,
		          row_number() OVER (
		            PARTITION BY LEAST(sc.id::text, tc.id::text), GREATEST(sc.id::text, tc.id::text), ls.relation
		            ORDER BY CASE ls.status
		              WHEN 'confirmed' THEN 0 WHEN 'relabeled' THEN 1 WHEN 'rejected' THEN 2
		              WHEN 'pending' THEN 3 ELSE 4 END,
		              ls.similarity DESC, ls.suggested_at DESC
		          ) AS duplicate_rank
		   FROM link_suggestions ls
		   JOIN chunks sc ON ls.source_chunk_id = sc.id
		   JOIN chunks tc ON ls.target_chunk_id = tc.id
		   JOIN documents sd ON sc.document_id = sd.id
		   JOIN documents td ON tc.document_id = td.id
		   WHERE ls.user_id = $1 AND ls.status NOT IN ('confirmed', 'relabeled')
		     AND sc.user_id = $1 AND tc.user_id = $1
		     AND sd.user_id = $1 AND td.user_id = $1
		     AND sd.id <> td.id
		     AND (sc.document_id = $2 OR tc.document_id = $2)
		 )
		 SELECT id, user_id, source_chunk_id, target_chunk_id,
		        similarity, relation, status, user_label,
		        time_to_respond_ms, suggested_at, responded_at,
		        src_text, tgt_text, src_document_id, tgt_document_id,
		        src_doc, tgt_doc, src_page, tgt_page,
		        summary, src_bboxes
		 FROM oriented
		 WHERE duplicate_rank = 1 AND ($3 = 0 OR src_page = $3)
		 ORDER BY similarity DESC`,
		userID, docID, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suggestions []model.LinkSuggestion
	for rows.Next() {
		var sg model.LinkSuggestion
		var srcBBoxes string
		if err := rows.Scan(
			&sg.ID, &sg.UserID, &sg.SourceChunkID, &sg.TargetChunkID,
			&sg.Similarity, &sg.Relation, &sg.Status, &sg.UserLabel,
			&sg.TimeToRespondMs, &sg.SuggestedAt, &sg.RespondedAt,
			&sg.SrcText, &sg.TgtText, &sg.SrcDocumentID, &sg.TgtDocumentID,
			&sg.SrcDoc, &sg.TgtDoc, &sg.SrcPage, &sg.TgtPage,
			&sg.Summary, &srcBBoxes,
		); err != nil {
			return nil, err
		}
		if srcBBoxes != "" {
			sg.SrcBBoxes = json.RawMessage(srcBBoxes)
		}
		suggestions = append(suggestions, sg)
	}
	return suggestions, rows.Err()
}

func (s *Store) RespondToSuggestion(ctx context.Context, userID, id string, action model.SuggestionStatus, label string, timeMs int) error {
	// Distance-only passage matches can be dismissed, never promoted to assertions.
	if action != model.SuggestionRejected {
		return fmt.Errorf("unclassified passage match cannot be confirmed as a relation")
	}
	now := time.Now()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	command, err := tx.Exec(ctx,
		`WITH selected AS (
		   SELECT source_chunk_id, target_chunk_id, relation
		   FROM link_suggestions WHERE id = $5 AND user_id = $6
		 )
		 UPDATE link_suggestions ls
		 SET status = $1,
		     user_label = COALESCE(NULLIF($2, ''), ls.user_label),
		     responded_at = $3,
		     time_to_respond_ms = $4
		 FROM selected s
		 WHERE ls.user_id = $6 AND ls.relation = s.relation
		   AND ((ls.source_chunk_id = s.source_chunk_id AND ls.target_chunk_id = s.target_chunk_id)
		     OR (ls.source_chunk_id = s.target_chunk_id AND ls.target_chunk_id = s.source_chunk_id))`,
		action, label, now, timeMs, id, userID)
	if err != nil {
		return fmt.Errorf("update suggestion state: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("suggestion not found")
	}

	payload, _ := json.Marshal(map[string]any{"action": action, "label": label, "time_to_respond_ms": timeMs})
	_, err = tx.Exec(ctx,
		`INSERT INTO learning_events (user_id, event_type, suggestion_id, payload, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, idempotency_key) DO NOTHING`,
		userID, "suggestion_"+string(action), id, payload, "suggestion:"+id+":"+string(action))
	if err != nil {
		return fmt.Errorf("record suggestion learning event: %w", err)
	}
	if action == model.SuggestionConfirmed || action == model.SuggestionRelabeled {
		_, err = tx.Exec(ctx,
			`INSERT INTO learner_concept_state (
			    user_id, concept_id, mastery_estimate, half_life_seconds,
			    last_retrieved_at, success_count, evidence_count, uncertainty
			 )
				 SELECT DISTINCT $1::uuid, cc.concept_id, 0.35, 129600, $3::timestamptz, 1, 1, 0.85
			 FROM link_suggestions ls
			 JOIN chunk_concepts cc ON cc.chunk_id IN (ls.source_chunk_id, ls.target_chunk_id)
				 WHERE ls.id = $2::uuid AND ls.user_id = $1::uuid
			 ON CONFLICT (user_id, concept_id) DO UPDATE SET
			    mastery_estimate = LEAST(0.99, learner_concept_state.mastery_estimate + 0.08),
			    half_life_seconds = LEAST(31536000, learner_concept_state.half_life_seconds * 1.5),
			    last_retrieved_at = EXCLUDED.last_retrieved_at,
			    success_count = learner_concept_state.success_count + 1,
			    evidence_count = learner_concept_state.evidence_count + 1,
			    uncertainty = GREATEST(0.05, learner_concept_state.uncertainty * 0.9),
			    updated_at = $3`, userID, id, now)
		if err != nil {
			return fmt.Errorf("update learner projection from suggestion: %w", err)
		}

		_, err = tx.Exec(ctx, `
			WITH suggestion AS (
			  SELECT ls.user_id, ls.id, ls.relation,
			         ls.source_chunk_id, ls.target_chunk_id,
			         sc.document_id AS source_document_id,
			         tc.document_id AS target_document_id
			  FROM link_suggestions ls
			  JOIN chunks sc ON sc.id = ls.source_chunk_id
			  JOIN chunks tc ON tc.id = ls.target_chunk_id
			  WHERE ls.id = $2::uuid AND ls.user_id = $1::uuid
			), concept_pairs AS (
			  SELECT DISTINCT s.user_id, s.id AS suggestion_id, s.relation,
			    CASE WHEN s.relation = 'related_to' AND source_cc.concept_id::text > target_cc.concept_id::text
			      THEN target_cc.concept_id ELSE source_cc.concept_id END AS source_concept_id,
			    CASE WHEN s.relation = 'related_to' AND source_cc.concept_id::text > target_cc.concept_id::text
			      THEN source_cc.concept_id ELSE target_cc.concept_id END AS target_concept_id,
			    CASE WHEN s.source_document_id = s.target_document_id THEN 1 ELSE 2 END AS document_count
			  FROM suggestion s
			  JOIN chunk_concepts source_cc ON source_cc.chunk_id = s.source_chunk_id
			  JOIN chunk_concepts target_cc ON target_cc.chunk_id = s.target_chunk_id
			  WHERE source_cc.concept_id <> target_cc.concept_id
			), reinforced AS (
			  INSERT INTO concept_edges (
			    user_id, source_concept_id, target_concept_id, relation,
			    created_via, state, confidence, confirmed_at,
			    base_confidence, evidence_confidence, support_count, document_count,
			    last_adapted_at, valid_from, observed_at
			  )
			  SELECT user_id, source_concept_id, target_concept_id, relation,
			         'user_confirmed', 'confirmed', 0.90, $3::timestamptz,
			         0.90, 0.90, 1, document_count,
			         $3::timestamptz, $3::timestamptz, $3::timestamptz
			  FROM concept_pairs
			  ON CONFLICT (source_concept_id, target_concept_id, relation) DO UPDATE SET
			    created_via = 'user_confirmed', state = 'confirmed',
			    confidence = GREATEST(concept_edges.confidence, 0.90),
			    base_confidence = GREATEST(concept_edges.base_confidence, 0.90),
			    evidence_confidence = GREATEST(concept_edges.evidence_confidence, 0.90),
			    support_count = concept_edges.support_count + 1,
			    document_count = GREATEST(concept_edges.document_count, EXCLUDED.document_count),
			    confirmed_at = $3::timestamptz, last_adapted_at = $3::timestamptz,
			    valid_to = NULL, observed_at = $3::timestamptz
			  RETURNING id
			)
			INSERT INTO learning_events (
			  user_id, event_type, suggestion_id, concept_edge_id, payload, source, idempotency_key
			)
			SELECT $1::uuid, 'suggestion_graph_confirmed', $2::uuid, id,
			       jsonb_build_object('confidence', 0.90), 'deterministic_reducer',
			       'suggestion-graph:' || $2::text || ':' || id::text
			FROM reinforced
			ON CONFLICT (user_id, idempotency_key) DO NOTHING`, userID, id, now)
		if err != nil {
			return fmt.Errorf("reinforce graph from suggestion: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// ============================================================
// Annotations
// ============================================================

func (s *Store) ListAnnotations(ctx context.Context, userID, docID string, page int) ([]model.Annotation, error) {
	q := `SELECT id, user_id, document_id, chunk_id, page, bbox, color, type, comment, created_at, updated_at
	      FROM annotations WHERE document_id = $1 AND user_id = $2`
	args := []any{docID, userID}
	if page > 0 {
		q += ` AND page = $3`
		args = append(args, page)
	}
	q += ` ORDER BY page, created_at`

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var anns []model.Annotation
	for rows.Next() {
		var a model.Annotation
		if err := rows.Scan(&a.ID, &a.UserID, &a.DocumentID, &a.ChunkID, &a.Page, &a.BBox, &a.Color, &a.Type, &a.Comment, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		anns = append(anns, a)
	}
	return anns, rows.Err()
}

func (s *Store) CreateAnnotation(ctx context.Context, a *model.Annotation) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if a.ChunkID == nil {
		var chunkID string
		if err := tx.QueryRow(ctx,
			`SELECT c.id FROM chunks c
			 JOIN documents d ON d.id = c.document_id
			 WHERE c.document_id = $1 AND c.user_id = $2 AND c.page_start <= $3 AND c.page_end >= $3
			 ORDER BY c.chunk_index LIMIT 1`, a.DocumentID, a.UserID, a.Page).Scan(&chunkID); err == nil {
			a.ChunkID = &chunkID
		}
	}

	if err := tx.QueryRow(ctx,
		`INSERT INTO annotations (user_id, document_id, chunk_id, page, bbox, color, type, comment)
		 SELECT $1, d.id, $3, $4, $5, $6, $7, $8
		 FROM documents d WHERE d.id = $2 AND d.user_id = $1
		 RETURNING id, created_at, updated_at`,
		a.UserID, a.DocumentID, a.ChunkID, a.Page, a.BBox, a.Color, a.Type, a.Comment,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{"annotation_id": a.ID, "annotation_type": a.Type, "page": a.Page, "has_comment": a.Comment != ""})
	eventType := "annotation_created"
	if a.Type == model.AnnotationNote {
		eventType = "note_created"
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO learning_events (user_id, event_type, document_id, chunk_id, payload, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		a.UserID, eventType, a.DocumentID, a.ChunkID, payload, "annotation:"+a.ID)
	if err != nil {
		return err
	}
	if a.ChunkID != nil {
		halfLifeMultiplier := 1.02
		masteryBoost := 0.01
		if a.Type == model.AnnotationNote {
			halfLifeMultiplier = 1.1
			masteryBoost = 0.03
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO learner_concept_state (
			    user_id, concept_id, mastery_estimate, half_life_seconds,
			    last_exposed_at, evidence_count, uncertainty
			 )
			 SELECT $1, cc.concept_id, 0.25 + $3::real, 86400 * $4::double precision,
			        now(), 1, 0.95
			 FROM chunk_concepts cc WHERE cc.chunk_id = $2
			 ON CONFLICT (user_id, concept_id) DO UPDATE SET
			    mastery_estimate = LEAST(0.99, learner_concept_state.mastery_estimate + $3::real),
			    half_life_seconds = LEAST(31536000, learner_concept_state.half_life_seconds * $4::double precision),
			    last_exposed_at = now(),
			    evidence_count = learner_concept_state.evidence_count + 1,
			    uncertainty = GREATEST(0.05, learner_concept_state.uncertainty * 0.97),
			    updated_at = now()`, a.UserID, *a.ChunkID, masteryBoost, halfLifeMultiplier)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteAnnotation(ctx context.Context, id, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM annotations WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// ============================================================
// Concepts & Graph
// ============================================================

func (s *Store) ListConcepts(ctx context.Context, userID string) ([]model.Concept, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, name, description, state, model_version, prompt_version, created_at
		 FROM concepts c WHERE user_id = $1 AND state NOT IN ('rejected', 'archived')
		 AND EXISTS (SELECT 1 FROM chunk_concepts cc WHERE cc.concept_id = c.id)
		 ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var concepts []model.Concept
	for rows.Next() {
		var c model.Concept
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Description, &c.State, &c.ModelVersion, &c.PromptVersion, &c.CreatedAt); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}
	return concepts, rows.Err()
}

func (s *Store) ListConceptEdges(ctx context.Context, userID string) ([]model.ConceptEdge, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, source_concept_id, target_concept_id, relation, created_via,
		        state, confidence, support_count, document_count, base_confidence, evidence_confidence,
		        confirmed_at, valid_from, valid_to, observed_at, COALESCE(superseded_by::text, ''), created_at
		 FROM concept_edges e WHERE user_id = $1
		 AND (e.created_via <> 'deterministic_chat' OR
		      (e.state = 'confirmed' AND EXISTS (
		        SELECT 1 FROM graph_edge_actions a
		        WHERE a.edge_id = e.id AND a.user_id = e.user_id AND a.action = 'confirmed')))
		 AND NOT EXISTS (SELECT 1 FROM learning_events le WHERE le.concept_edge_id = e.id AND le.user_id = $1 AND le.event_type IN ('mental_link_graph_confirmed', 'suggestion_graph_confirmed'))
		 AND EXISTS (SELECT 1 FROM chunk_concepts cc WHERE cc.concept_id = e.source_concept_id)
		 AND EXISTS (SELECT 1 FROM chunk_concepts cc WHERE cc.concept_id = e.target_concept_id)
		 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []model.ConceptEdge
	for rows.Next() {
		var e model.ConceptEdge
		if err := rows.Scan(&e.ID, &e.UserID, &e.SourceConceptID, &e.TargetConceptID, &e.Relation,
			&e.CreatedVia, &e.State, &e.Confidence, &e.SupportCount, &e.DocumentCount,
			&e.BaseConfidence, &e.EvidenceConfidence, &e.ConfirmedAt, &e.ValidFrom, &e.ValidTo,
			&e.ObservedAt, &e.SupersededBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// ListDocumentConceptEdges connects each latest document mental model to the
// concepts that have evidence in that document. This keeps the unified graph
// grounded in stored chunk evidence rather than adding another model call.
func (s *Store) ListDocumentConceptEdges(ctx context.Context, userID string) ([]model.GraphEdge, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT mm.id, c.id
		 FROM document_mental_models mm
		 JOIN chunks ch ON ch.document_id = mm.document_id
		 JOIN chunk_concepts cc ON cc.chunk_id = ch.id
		 JOIN concepts c ON c.id = cc.concept_id AND c.user_id = mm.user_id
		 WHERE mm.user_id = $1 AND mm.status = 'ready'
		   AND mm.version = (
		     SELECT MAX(latest.version) FROM document_mental_models latest
		     WHERE latest.document_id = mm.document_id AND latest.status = 'ready'
		   )
		   AND c.state NOT IN ('rejected', 'archived')
		 ORDER BY mm.id, c.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []model.GraphEdge
	for rows.Next() {
		var mentalModelID, conceptID string
		if err := rows.Scan(&mentalModelID, &conceptID); err != nil {
			return nil, err
		}
		edges = append(edges, model.GraphEdge{
			ID:         "uses-concept:" + mentalModelID + ":" + conceptID,
			Source:     "model:" + mentalModelID,
			Target:     conceptID,
			Relation:   "uses_concept",
			State:      "supported",
			Confidence: 1,
			CreatedVia: "system",
		})
	}
	return edges, rows.Err()
}

// ListReviewedSuggestionEdges projects passage-level user decisions between
// document mental models. A suggestion remains visible in the adaptive graph
// even when either passage has no extracted concept assignment; this preserves
// the user's evidence-backed decision without manufacturing concept nodes.
func (s *Store) ListReviewedSuggestionEdges(ctx context.Context, userID string) ([]model.GraphEdge, error) {
	rows, err := s.pool.Query(ctx, `
		WITH reviewed AS (
		  SELECT DISTINCT ON (
		    LEAST(ls.source_chunk_id::text, ls.target_chunk_id::text),
		    GREATEST(ls.source_chunk_id::text, ls.target_chunk_id::text),
		    ls.relation
		  ) ls.id, ls.source_chunk_id, ls.target_chunk_id, ls.relation,
		    ls.user_label, ls.status, ls.similarity, ls.summary,
		    COALESCE(ls.responded_at, ls.suggested_at) AS observed_at
		  FROM link_suggestions ls
		  WHERE ls.user_id = $1 AND ls.evidence_verified
		    AND ls.status IN ('confirmed', 'relabeled', 'rejected')
		  ORDER BY LEAST(ls.source_chunk_id::text, ls.target_chunk_id::text),
		    GREATEST(ls.source_chunk_id::text, ls.target_chunk_id::text),
		    ls.relation, ls.responded_at DESC NULLS LAST, ls.id
		)
		SELECT r.id, source_model.id, target_model.id,
		       COALESCE(NULLIF(r.user_label, ''), r.relation), r.status,
		       r.similarity, r.summary, r.observed_at
		FROM reviewed r
		JOIN chunks source_chunk ON source_chunk.id = r.source_chunk_id
		JOIN chunks target_chunk ON target_chunk.id = r.target_chunk_id
		JOIN LATERAL (
		  SELECT mm.id FROM document_mental_models mm
		  WHERE mm.document_id = source_chunk.document_id AND mm.user_id = $1 AND mm.status = 'ready'
		  ORDER BY mm.version DESC LIMIT 1
		) source_model ON true
		JOIN LATERAL (
		  SELECT mm.id FROM document_mental_models mm
		  WHERE mm.document_id = target_chunk.document_id AND mm.user_id = $1 AND mm.status = 'ready'
		  ORDER BY mm.version DESC LIMIT 1
		) target_model ON true
		WHERE source_model.id <> target_model.id
		ORDER BY r.observed_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	edges := []model.GraphEdge{}
	for rows.Next() {
		var id, sourceModelID, targetModelID, relation, status, summary string
		var confidence float32
		var observedAt time.Time
		if err := rows.Scan(&id, &sourceModelID, &targetModelID, &relation, &status,
			&confidence, &summary, &observedAt); err != nil {
			return nil, err
		}
		state := "confirmed"
		if status == string(model.SuggestionRejected) {
			state = "rejected"
		}
		if summary == "" {
			summary = "You reviewed a passage connection between these documents."
		}
		edges = append(edges, model.GraphEdge{
			ID: "suggestion:" + id, Source: "model:" + sourceModelID, Target: "model:" + targetModelID,
			Relation: relation, State: state, Confidence: confidence, CreatedVia: "user_reviewed",
			Explanation: summary, ValidFrom: &observedAt, ObservedAt: &observedAt,
		})
	}
	return edges, rows.Err()
}

// ============================================================
// Reading Sessions
// ============================================================

func (s *Store) CreateReadingSession(ctx context.Context, sess *model.ReadingSession) error {
	return s.pool.QueryRow(ctx,
		`INSERT INTO reading_sessions (user_id, document_id, started_at)
		 SELECT $1, d.id, $3 FROM documents d WHERE d.id = $2 AND d.user_id = $1
		 RETURNING id`,
		sess.UserID, sess.DocumentID, sess.StartedAt,
	).Scan(&sess.ID)
}

func (s *Store) EndReadingSession(ctx context.Context, userID, id string, pagesViewed []int, maxDepth int) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE reading_sessions SET ended_at = $1, pages_viewed = $2, max_scroll_depth = $3
		 WHERE id = $4 AND user_id = $5`,
		now, pagesViewed, maxDepth, id, userID)
	return err
}

// ============================================================
// Runtime Mental Models
// ============================================================

func (s *Store) GetDocumentMentalModel(ctx context.Context, userID, documentID string) (*model.DocumentMentalModel, error) {
	mentalModel := &model.DocumentMentalModel{}
	err := s.pool.QueryRow(ctx,
		`SELECT mm.id, mm.document_id, mm.user_id, d.title, mm.version, mm.main_claim,
		        mm.key_concepts, mm.assumptions, mm.open_questions, mm.domain,
		        mm.model_version, mm.prompt_version, mm.status, mm.generated_at
		 FROM document_mental_models mm
		 JOIN documents d ON d.id = mm.document_id
		 WHERE mm.user_id = $1 AND mm.document_id = $2 AND mm.status = 'ready'
		 ORDER BY mm.version DESC LIMIT 1`, userID, documentID).Scan(
		&mentalModel.ID, &mentalModel.DocumentID, &mentalModel.UserID, &mentalModel.DocumentTitle,
		&mentalModel.Version, &mentalModel.MainClaim, &mentalModel.KeyConcepts, &mentalModel.Assumptions,
		&mentalModel.OpenQuestions, &mentalModel.Domain, &mentalModel.ModelVersion,
		&mentalModel.PromptVersion, &mentalModel.Status, &mentalModel.GeneratedAt,
	)
	if err != nil {
		return nil, err
	}
	return mentalModel, nil
}

func (s *Store) ListMentalModels(ctx context.Context, userID string) ([]model.DocumentMentalModel, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT ON (mm.document_id)
		        mm.id, mm.document_id, mm.user_id, d.title, mm.version, mm.main_claim,
		        mm.key_concepts, mm.assumptions, mm.open_questions, mm.domain,
		        mm.model_version, mm.prompt_version, mm.status, mm.generated_at
		 FROM document_mental_models mm
		 JOIN documents d ON d.id = mm.document_id
		 WHERE mm.user_id = $1 AND mm.status = 'ready'
		 ORDER BY mm.document_id, mm.version DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []model.DocumentMentalModel
	for rows.Next() {
		var mentalModel model.DocumentMentalModel
		if err := rows.Scan(&mentalModel.ID, &mentalModel.DocumentID, &mentalModel.UserID,
			&mentalModel.DocumentTitle, &mentalModel.Version, &mentalModel.MainClaim,
			&mentalModel.KeyConcepts, &mentalModel.Assumptions, &mentalModel.OpenQuestions,
			&mentalModel.Domain, &mentalModel.ModelVersion, &mentalModel.PromptVersion,
			&mentalModel.Status, &mentalModel.GeneratedAt); err != nil {
			return nil, err
		}
		models = append(models, mentalModel)
	}
	return models, rows.Err()
}

func (s *Store) ListMentalModelLinks(ctx context.Context, userID, documentID string) ([]model.MentalModelLink, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT ml.id, ml.user_id, ml.source_model_id, ml.target_model_id,
		        sm.document_id, tm.document_id, sd.title, td.title, ml.link_type,
		        ml.similarity, ml.confidence, ml.bridge_explanation,
		        ml.source_evidence_chunk_id, ml.target_evidence_chunk_id,
		        COALESCE(sc.content, ''), COALESCE(tc.content, ''),
		        ml.source_evidence->>'quote', ml.target_evidence->>'quote',
		        ml.source_evidence->'locator', ml.target_evidence->'locator',
		        ml.status, ml.created_via, ml.model_version, ml.prompt_version,
		        ml.user_label, ml.suggested_at, ml.responded_at, ml.review_revision
		 FROM mental_model_links ml
		 JOIN document_mental_models sm ON sm.id = ml.source_model_id
		 JOIN document_mental_models tm ON tm.id = ml.target_model_id
		 JOIN documents sd ON sd.id = sm.document_id
		 JOIN documents td ON td.id = tm.document_id
		 LEFT JOIN chunks sc ON sc.id = ml.source_evidence_chunk_id
		 LEFT JOIN chunks tc ON tc.id = ml.target_evidence_chunk_id
		 WHERE ml.user_id = $1
		   AND ($2 = '' OR sm.document_id::text = $2 OR tm.document_id::text = $2)
		   AND (ml.status != 'archived' OR ml.review_revision > 0)
		   AND sd.status = 'ready' AND td.status = 'ready'
		   AND valid_grounded_mental_link(ml)
		   AND NOT EXISTS (SELECT 1 FROM document_mental_models newer WHERE newer.document_id = sm.document_id AND newer.version > sm.version AND newer.status = 'ready')
		   AND NOT EXISTS (SELECT 1 FROM document_mental_models newer WHERE newer.document_id = tm.document_id AND newer.version > tm.version AND newer.status = 'ready')
		 ORDER BY CASE ml.status WHEN 'candidate' THEN 0 WHEN 'confirmed' THEN 1 ELSE 2 END,
		          ml.confidence DESC, ml.suggested_at DESC`, userID, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []model.MentalModelLink
	for rows.Next() {
		var link model.MentalModelLink
		if err := rows.Scan(&link.ID, &link.UserID, &link.SourceModelID, &link.TargetModelID,
			&link.SourceDocumentID, &link.TargetDocumentID, &link.SourceDocumentTitle,
			&link.TargetDocumentTitle, &link.LinkType, &link.Similarity, &link.Confidence,
			&link.BridgeExplanation, &link.SourceEvidenceChunkID, &link.TargetEvidenceChunkID,
			&link.SourceEvidence, &link.TargetEvidence,
			&link.SourceQuote, &link.TargetQuote, &link.SourceLocator, &link.TargetLocator,
			&link.Status, &link.CreatedVia,
			&link.ModelVersion, &link.PromptVersion, &link.UserLabel, &link.SuggestedAt,
			&link.RespondedAt, &link.Revision); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) ListLearnerConceptState(ctx context.Context, userID string) ([]model.LearnerConceptState, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT state.user_id, state.concept_id, c.name, state.mastery_estimate,
		        state.half_life_seconds, state.last_exposed_at, state.last_retrieved_at,
		        state.success_count, state.failure_count, state.evidence_count,
		        state.uncertainty, state.state_version, state.updated_at
		 FROM learner_concept_state state
		 JOIN concepts c ON c.id = state.concept_id
		 WHERE state.user_id = $1 ORDER BY state.updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []model.LearnerConceptState
	now := time.Now()
	for rows.Next() {
		var state model.LearnerConceptState
		if err := rows.Scan(&state.UserID, &state.ConceptID, &state.ConceptName,
			&state.MasteryEstimate, &state.HalfLifeSeconds, &state.LastExposedAt,
			&state.LastRetrievedAt, &state.SuccessCount, &state.FailureCount,
			&state.EvidenceCount, &state.Uncertainty, &state.StateVersion,
			&state.UpdatedAt); err != nil {
			return nil, err
		}
		anchor := state.UpdatedAt
		if state.LastRetrievedAt != nil {
			anchor = *state.LastRetrievedAt
		} else if state.LastExposedAt != nil {
			anchor = *state.LastExposedAt
		}
		elapsed := math.Max(0, now.Sub(anchor).Seconds())
		state.RecallProbability = math.Pow(2, -elapsed/math.Max(1, state.HalfLifeSeconds))
		states = append(states, state)
	}
	return states, rows.Err()
}
