// Package store provides the Postgres data-access layer for SELAR.
// All queries use pgx directly (no ORM) against the schema defined
// in migrations/001_init.sql — users, documents, chunks, link_suggestions,
// concepts, concept_edges, annotations, reading_sessions, quizzes.
package store

import (
	"context"
	"encoding/json"
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
		        sd.title AS src_doc, td.title AS tgt_doc, tc.page_start AS tgt_page
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
		if err := rows.Scan(
			&sg.ID, &sg.UserID, &sg.SourceChunkID, &sg.TargetChunkID,
			&sg.Similarity, &sg.Relation, &sg.Status, &sg.UserLabel,
			&sg.TimeToRespondMs, &sg.SuggestedAt, &sg.RespondedAt,
			&sg.SrcText, &sg.TgtText, &sg.SrcDoc, &sg.TgtDoc, &sg.TgtPage,
		); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, sg)
	}
	return suggestions, rows.Err()
}

func (s *Store) RespondToSuggestion(ctx context.Context, id string, action model.SuggestionStatus, label string, timeMs int) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE link_suggestions
		 SET status = $1,
		     user_label = COALESCE(NULLIF($2, ''), user_label),
		     responded_at = $3,
		     time_to_respond_ms = $4
		 WHERE id = $5`,
		action, label, now, timeMs, id)
	return err
}

// ============================================================
// Annotations
// ============================================================

func (s *Store) ListAnnotations(ctx context.Context, docID string, page int) ([]model.Annotation, error) {
	q := `SELECT id, user_id, document_id, chunk_id, page, bbox, color, type, comment, created_at, updated_at
	      FROM annotations WHERE document_id = $1`
	args := []any{docID}
	if page > 0 {
		q += ` AND page = $2`
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
	return s.pool.QueryRow(ctx,
		`INSERT INTO annotations (user_id, document_id, chunk_id, page, bbox, color, type, comment)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at, updated_at`,
		a.UserID, a.DocumentID, a.ChunkID, a.Page, a.BBox, a.Color, a.Type, a.Comment,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
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
		`SELECT id, user_id, name, description, created_at FROM concepts WHERE user_id = $1 ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var concepts []model.Concept
	for rows.Next() {
		var c model.Concept
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Description, &c.CreatedAt); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}
	return concepts, rows.Err()
}

func (s *Store) ListConceptEdges(ctx context.Context, userID string) ([]model.ConceptEdge, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, source_concept_id, target_concept_id, relation, created_via, confirmed_at, created_at
		 FROM concept_edges WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []model.ConceptEdge
	for rows.Next() {
		var e model.ConceptEdge
		if err := rows.Scan(&e.ID, &e.UserID, &e.SourceConceptID, &e.TargetConceptID, &e.Relation, &e.CreatedVia, &e.ConfirmedAt, &e.CreatedAt); err != nil {
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
		 VALUES ($1, $2, $3) RETURNING id`,
		sess.UserID, sess.DocumentID, sess.StartedAt,
	).Scan(&sess.ID)
}

func (s *Store) EndReadingSession(ctx context.Context, id string, pagesViewed []int, maxDepth int) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE reading_sessions SET ended_at = $1, pages_viewed = $2, max_scroll_depth = $3 WHERE id = $4`,
		now, pagesViewed, maxDepth, id)
	return err
}
