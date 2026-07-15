// Package store provides the Postgres data-access layer for SELAR.
// All queries use pgx directly (no ORM) against the schema defined
// in migrations/001_init.sql — users, documents, chunks, link_suggestions,
// concepts, concept_edges, annotations, reading_sessions, quizzes.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/selar-dev/selar-api/internal/model"
)

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
		`SELECT id, user_id, title, authors, year, page_count, status, progress, added_at, processed_at
		 FROM documents WHERE user_id = $1 ORDER BY added_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []model.Document
	for rows.Next() {
		var d model.Document
		if err := rows.Scan(&d.ID, &d.UserID, &d.Title, &d.Authors, &d.Year, &d.PageCount, &d.Status, &d.Progress, &d.AddedAt, &d.ProcessedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func (s *Store) GetDocument(ctx context.Context, id, userID string) (*model.Document, error) {
	d := &model.Document{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, title, authors, year, page_count, status, progress, added_at, processed_at
		 FROM documents WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&d.ID, &d.UserID, &d.Title, &d.Authors, &d.Year, &d.PageCount, &d.Status, &d.Progress, &d.AddedAt, &d.ProcessedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Store) CreateDocument(ctx context.Context, d *model.Document) error {
	return s.pool.QueryRow(ctx,
		`INSERT INTO documents (user_id, title, authors, year, page_count, status, file_path, gdrive_file_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, added_at`,
		d.UserID, d.Title, d.Authors, d.Year, d.PageCount, d.Status, d.FilePath, d.GDriveFileID,
	).Scan(&d.ID, &d.AddedAt)
}

func (s *Store) DeleteDocument(ctx context.Context, id, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (s *Store) GetDocumentStats(ctx context.Context, userID string) (*model.DocumentStats, error) {
	stats := &model.DocumentStats{}
	err := s.pool.QueryRow(ctx,
		`SELECT
		   COUNT(DISTINCT d.id),
		   COALESCE(SUM((SELECT COUNT(*) FROM chunks c WHERE c.document_id = d.id)), 0),
		   COALESCE((SELECT COUNT(*) FROM link_suggestions ls
		     WHERE ls.user_id = $1 AND ls.status = 'confirmed'), 0)
		 FROM documents d WHERE d.user_id = $1`,
		userID,
	).Scan(&stats.TotalDocuments, &stats.TotalChunks, &stats.ConfirmedLinks)
	return stats, err
}

// ============================================================
// Link Suggestions
// ============================================================

func (s *Store) ListSuggestions(ctx context.Context, userID, docID string, page int) ([]model.LinkSuggestion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT ls.id, ls.user_id, ls.source_chunk_id, ls.target_chunk_id,
		        ls.similarity, ls.relation, ls.status, ls.user_label,
		        ls.time_to_respond_ms, ls.suggested_at, ls.responded_at,
		        sc.content AS src_text, tc.content AS tgt_text,
		        sd.title AS src_doc, td.title AS tgt_doc, tc.page_start AS tgt_page,
		        ls.summary, sc.bboxes AS src_bboxes
		 FROM link_suggestions ls
		 JOIN chunks sc ON ls.source_chunk_id = sc.id
		 JOIN chunks tc ON ls.target_chunk_id = tc.id
		 JOIN documents sd ON sc.document_id = sd.id
		 JOIN documents td ON tc.document_id = td.id
		 WHERE ls.user_id = $1
		   AND sc.document_id = $2
		   AND ($3 = 0 OR sc.page_start = $3)
		 ORDER BY ls.similarity DESC`,
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
			&sg.SrcText, &sg.TgtText, &sg.SrcDoc, &sg.TgtDoc, &sg.TgtPage,
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
	now := time.Now()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	command, err := tx.Exec(ctx,
		`UPDATE link_suggestions
		 SET status = $1,
		     user_label = COALESCE(NULLIF($2, ''), user_label),
		     responded_at = $3,
		     time_to_respond_ms = $4
		 WHERE id = $5 AND user_id = $6`,
		action, label, now, timeMs, id, userID)
	if err != nil {
		return err
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
		return err
	}
	if action == model.SuggestionConfirmed || action == model.SuggestionRelabeled {
		_, err = tx.Exec(ctx,
			`INSERT INTO learner_concept_state (
			    user_id, concept_id, mastery_estimate, half_life_seconds,
			    last_retrieved_at, success_count, evidence_count, uncertainty
			 )
				 SELECT DISTINCT $1, cc.concept_id, 0.35, 129600, $3, 1, 1, 0.85
			 FROM link_suggestions ls
			 JOIN chunk_concepts cc ON cc.chunk_id IN (ls.source_chunk_id, ls.target_chunk_id)
			 WHERE ls.id = $2 AND ls.user_id = $1
			 ON CONFLICT (user_id, concept_id) DO UPDATE SET
			    mastery_estimate = LEAST(0.99, learner_concept_state.mastery_estimate + 0.08),
			    half_life_seconds = LEAST(31536000, learner_concept_state.half_life_seconds * 1.5),
			    last_retrieved_at = EXCLUDED.last_retrieved_at,
			    success_count = learner_concept_state.success_count + 1,
			    evidence_count = learner_concept_state.evidence_count + 1,
			    uncertainty = GREATEST(0.05, learner_concept_state.uncertainty * 0.9),
			    updated_at = $3`, userID, id, now)
		if err != nil {
			return err
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
		 FROM concepts WHERE user_id = $1 AND state NOT IN ('rejected', 'archived') ORDER BY name`, userID)
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
		        state, confidence, confirmed_at, created_at
		 FROM concept_edges WHERE user_id = $1 AND state NOT IN ('rejected', 'archived') ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []model.ConceptEdge
	for rows.Next() {
		var e model.ConceptEdge
		if err := rows.Scan(&e.ID, &e.UserID, &e.SourceConceptID, &e.TargetConceptID, &e.Relation, &e.CreatedVia, &e.State, &e.Confidence, &e.ConfirmedAt, &e.CreatedAt); err != nil {
			return nil, err
		}
		edges = append(edges, e)
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
		        ml.status, ml.created_via, ml.model_version, ml.prompt_version,
		        ml.user_label, ml.suggested_at, ml.responded_at
		 FROM mental_model_links ml
		 JOIN document_mental_models sm ON sm.id = ml.source_model_id
		 JOIN document_mental_models tm ON tm.id = ml.target_model_id
		 JOIN documents sd ON sd.id = sm.document_id
		 JOIN documents td ON td.id = tm.document_id
		 LEFT JOIN chunks sc ON sc.id = ml.source_evidence_chunk_id
		 LEFT JOIN chunks tc ON tc.id = ml.target_evidence_chunk_id
		 WHERE ml.user_id = $1
		   AND ($2 = '' OR sm.document_id::text = $2 OR tm.document_id::text = $2)
		   AND ml.status != 'archived'
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
			&link.SourceEvidence, &link.TargetEvidence, &link.Status, &link.CreatedVia,
			&link.ModelVersion, &link.PromptVersion, &link.UserLabel, &link.SuggestedAt,
			&link.RespondedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) RespondToMentalModelLink(ctx context.Context, userID, id string, response model.MentalModelLinkResponse) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	createdVia := model.EdgeAISuggested
	if response.Action == model.MentalLinkConfirmed || response.Action == model.MentalLinkRelabeled {
		createdVia = model.EdgeUserConfirmed
	}
	command, err := tx.Exec(ctx,
		`UPDATE mental_model_links
		 SET status = $1, user_label = COALESCE(NULLIF($2, ''), user_label),
		     created_via = $3, responded_at = now()
		 WHERE id = $4 AND user_id = $5`,
		response.Action, response.Label, createdVia, id, userID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("mental-model link not found")
	}

	payload, _ := json.Marshal(map[string]any{"action": response.Action, "label": response.Label})
	_, err = tx.Exec(ctx,
		`INSERT INTO learning_events (user_id, event_type, mental_link_id, payload, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, idempotency_key) DO NOTHING`,
		userID, "mental_link_"+string(response.Action), id, payload,
		"mental-link:"+id+":"+string(response.Action))
	if err != nil {
		return err
	}
	if response.Action == model.MentalLinkConfirmed || response.Action == model.MentalLinkRelabeled {
		_, err = tx.Exec(ctx,
			`INSERT INTO learner_concept_state (
			    user_id, concept_id, mastery_estimate, half_life_seconds,
			    last_retrieved_at, success_count, evidence_count, uncertainty
			 )
				 SELECT DISTINCT $1, cc.concept_id, 0.38, 172800, now(), 1, 1, 0.8
			 FROM mental_model_links ml
			 JOIN chunk_concepts cc ON cc.chunk_id IN (ml.source_evidence_chunk_id, ml.target_evidence_chunk_id)
			 WHERE ml.id = $2 AND ml.user_id = $1
			 ON CONFLICT (user_id, concept_id) DO UPDATE SET
			    mastery_estimate = LEAST(0.99, learner_concept_state.mastery_estimate + 0.1),
			    half_life_seconds = LEAST(31536000, learner_concept_state.half_life_seconds * 1.7),
			    last_retrieved_at = now(),
			    success_count = learner_concept_state.success_count + 1,
			    evidence_count = learner_concept_state.evidence_count + 1,
			    uncertainty = GREATEST(0.05, learner_concept_state.uncertainty * 0.85),
			    updated_at = now()`, userID, id)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
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
