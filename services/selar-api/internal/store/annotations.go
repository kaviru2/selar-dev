package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/model"
)

var ErrAnnotationSourceChanged = errors.New("document source changed; annotation was not modified")

func lockedAnnotation(ctx context.Context, tx pgx.Tx, id, user string) (*model.Annotation, error) {
	var a model.Annotation
	var source string
	err := tx.QueryRow(ctx, `SELECT a.id,a.user_id,a.document_id,a.chunk_id,a.page,a.bbox,a.color,a.type,COALESCE(a.comment,''),a.created_at,a.updated_at,a.anchor,COALESCE(d.content_hash,'')
 FROM annotations a JOIN documents d ON d.id=a.document_id AND d.user_id=a.user_id
 WHERE a.id=$1 AND a.user_id=$2 FOR UPDATE OF a FOR SHARE OF d`, id, user).Scan(&a.ID, &a.UserID, &a.DocumentID, &a.ChunkID, &a.Page, &a.BBox, &a.Color, &a.Type, &a.Comment, &a.CreatedAt, &a.UpdatedAt, &a.Anchor, &source)
	if err != nil {
		return nil, err
	}
	if a.Anchor != nil && (source == "" || a.Anchor.SourceHash != source) {
		return nil, ErrAnnotationSourceChanged
	}
	return &a, nil
}

// UpdateAnnotation permits only presentation edits. Source, page, quote, geometry,
// owner and chunk identity remain immutable and edits do not award learning credit.
func (s *Store) UpdateAnnotation(ctx context.Context, id, user string, patch model.AnnotationPatch) (*model.Annotation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	a, err := lockedAnnotation(ctx, tx, id, user)
	if err != nil {
		return nil, err
	}
	if a.Anchor != nil && patch.SourceHash != a.Anchor.SourceHash {
		return nil, ErrAnnotationSourceChanged
	}
	if patch.Color != nil {
		a.Color = *patch.Color
	}
	if patch.Comment != nil {
		a.Comment = *patch.Comment
	}
	err = tx.QueryRow(ctx, `UPDATE annotations SET color=$3,comment=$4,updated_at=now() WHERE id=$1 AND user_id=$2 RETURNING updated_at`, id, user, a.Color, a.Comment).Scan(&a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) DeleteAnnotation(ctx context.Context, id, user string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = lockedAnnotation(ctx, tx, id, user); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM annotations WHERE id=$1 AND user_id=$2`, id, user); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
