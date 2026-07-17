package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/selar-dev/selar-api/internal/model"
)

func (s *Store) CreateSource(ctx context.Context, source *model.ContentSource) error {
	if source.RefreshPolicy == "" {
		source.RefreshPolicy = "manual"
	}
	if source.Status == "" {
		source.Status = "active"
	}
	if len(source.Config) == 0 {
		source.Config = json.RawMessage(`{}`)
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO content_sources (
		    user_id, kind, uri, canonical_uri, title, refresh_policy, status, config
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		source.UserID, source.Kind, source.URI, source.CanonicalURI, source.Title,
		source.RefreshPolicy, source.Status, source.Config,
	).Scan(&source.ID, &source.CreatedAt, &source.UpdatedAt)
}

func (s *Store) GetSource(ctx context.Context, id, userID string) (*model.ContentSource, error) {
	source := &model.ContentSource{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, kind, uri, canonical_uri, title, refresh_policy, status,
		       config, last_content_hash, last_fetched_at, last_error, created_at, updated_at
		FROM content_sources WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&source.ID, &source.UserID, &source.Kind, &source.URI, &source.CanonicalURI,
		&source.Title, &source.RefreshPolicy, &source.Status, &source.Config,
		&source.LastContentHash, &source.LastFetchedAt, &source.LastError,
		&source.CreatedAt, &source.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return source, nil
}

func (s *Store) FindSourceByCanonicalURI(ctx context.Context, userID, canonicalURI string) (*model.ContentSource, error) {
	source := &model.ContentSource{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, kind, uri, canonical_uri, title, refresh_policy, status,
		       config, last_content_hash, last_fetched_at, last_error, created_at, updated_at
		FROM content_sources
		WHERE user_id = $1 AND canonical_uri = $2 AND status <> 'archived'`, userID, canonicalURI,
	).Scan(&source.ID, &source.UserID, &source.Kind, &source.URI, &source.CanonicalURI,
		&source.Title, &source.RefreshPolicy, &source.Status, &source.Config,
		&source.LastContentHash, &source.LastFetchedAt, &source.LastError,
		&source.CreatedAt, &source.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return source, nil
}

func (s *Store) ListSources(ctx context.Context, userID string) ([]model.ContentSource, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, kind, uri, canonical_uri, title, refresh_policy, status,
		       config, last_content_hash, last_fetched_at, last_error, created_at, updated_at
		FROM content_sources WHERE user_id = $1 AND status <> 'archived'
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []model.ContentSource{}
	for rows.Next() {
		var source model.ContentSource
		if err := rows.Scan(&source.ID, &source.UserID, &source.Kind, &source.URI,
			&source.CanonicalURI, &source.Title, &source.RefreshPolicy, &source.Status,
			&source.Config, &source.LastContentHash, &source.LastFetchedAt, &source.LastError,
			&source.CreatedAt, &source.UpdatedAt); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *Store) CreateIngestionRun(ctx context.Context, run *model.IngestionRun, userID string) error {
	if run.EmbeddingDimension == 0 {
		run.EmbeddingDimension = 3072
	}
	if len(run.Metrics) == 0 {
		run.Metrics = json.RawMessage(`{}`)
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO ingestion_runs (
		    source_id, document_id, user_id, status, embedding_model, embedding_dimension, metrics
		) VALUES ($1, $2, $3, 'queued', $4, $5, $6)
		RETURNING id, status, created_at`, run.SourceID, run.DocumentID, userID,
		run.EmbeddingModel, run.EmbeddingDimension, run.Metrics,
	).Scan(&run.ID, &run.Status, &run.CreatedAt)
}

func (s *Store) ListIngestionRuns(ctx context.Context, sourceID, userID string) ([]model.IngestionRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, source_id, document_id, status, extractor, extractor_version,
		       embedding_model, embedding_dimension, metrics, error, started_at, completed_at, created_at
		FROM ingestion_runs WHERE source_id = $1 AND user_id = $2
		ORDER BY created_at DESC`, sourceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []model.IngestionRun{}
	for rows.Next() {
		var run model.IngestionRun
		if err := rows.Scan(&run.ID, &run.SourceID, &run.DocumentID, &run.Status,
			&run.Extractor, &run.ExtractorVersion, &run.EmbeddingModel,
			&run.EmbeddingDimension, &run.Metrics, &run.Error, &run.StartedAt,
			&run.CompletedAt, &run.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) ArchiveSource(ctx context.Context, id, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE content_sources SET status = 'archived', updated_at = $3
		WHERE id = $1 AND user_id = $2`, id, userID, time.Now())
	return err
}
