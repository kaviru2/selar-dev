package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/model"
	"strings"
)

var ErrReviewStale = errors.New("stale review revision")
var ErrReviewInvalid = errors.New("invalid review action")

// RespondToMentalModelLink serializes per-link decisions with a row lock and
// rechecks both witnesses while locking the underlying source snapshots.
// A review click never records retrieval success or writes a concept edge.
func (s *Store) RespondToMentalModelLink(ctx context.Context, owner, id string, response model.MentalModelLinkResponse) error {
	if response.Revision == nil || *response.Revision < 0 {
		return ErrReviewInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status model.MentalLinkStatus
	var label *string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT status,user_label,review_revision FROM mental_model_links WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, owner).Scan(&status, &label, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMentalModelLinkNotFound
	}
	if err != nil {
		return err
	}
	if revision != *response.Revision {
		return ErrReviewStale
	}
	// FOR SHARE prevents a concurrent document/chunk/model rewrite between the
	// live witness check and this transaction's decision commit.
	rows, err := tx.Query(ctx, `SELECT d.id FROM mental_model_links ml
 JOIN document_mental_models mm ON mm.id IN (ml.source_model_id,ml.target_model_id)
 JOIN documents d ON d.id=mm.document_id
 WHERE ml.id=$1 AND ml.user_id=$2 FOR SHARE OF d`, id, owner)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `SELECT c.id FROM mental_model_links ml
 JOIN chunks c ON c.id IN (ml.source_evidence_chunk_id,ml.target_evidence_chunk_id)
 WHERE ml.id=$1 AND ml.user_id=$2 FOR SHARE OF c`, id, owner)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT valid_grounded_mental_link(ml) FROM mental_model_links ml WHERE ml.id=$1 AND ml.user_id=$2`, id, owner).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrMentalModelLinkNotFound
	}
	next, err := planGroundedReview(status, label, response)
	if err != nil {
		return err
	}
	var nextLabel *string = label
	if response.Action == model.MentalLinkRelabeled {
		corrected := strings.TrimSpace(response.Label)
		nextLabel = &corrected
	}
	if response.Action == "rolled_back" {
		if *response.TargetRevision != revision-1 {
			return ErrReviewInvalid
		}
		if *response.TargetRevision == 0 {
			next = model.MentalLinkCandidate
			nextLabel = nil
		} else {
			err = tx.QueryRow(ctx, `SELECT after_status,after_label FROM mental_link_review_events WHERE link_id=$1 AND user_id=$2 AND revision=$3`, id, owner, *response.TargetRevision).Scan(&next, &nextLabel)
			if err != nil {
				return err
			}
		}
		if next == status && ((nextLabel == nil && label == nil) || (nextLabel != nil && label != nil && *nextLabel == *label)) {
			return ErrReviewInvalid
		}
	}
	via := "ai_suggested"
	if next == model.MentalLinkConfirmed || next == model.MentalLinkRelabeled {
		via = "user_confirmed"
	}
	command, err := tx.Exec(ctx, `UPDATE mental_model_links SET status=$1,user_label=$2,created_via=$3,responded_at=now(),review_revision=review_revision+1 WHERE id=$4 AND user_id=$5 AND review_revision=$6 AND valid_grounded_mental_link(mental_model_links)`, next, nextLabel, via, id, owner, revision)
	if err != nil {
		return fmt.Errorf("save reviewed link: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrReviewStale
	}
	_, err = tx.Exec(ctx, `INSERT INTO mental_link_review_events (user_id,link_id,revision,action,before_status,after_status,before_label,after_label,reason,target_revision)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, owner, id, revision+1, response.Action, status, next, label, nextLabel, strings.TrimSpace(response.Reason), response.TargetRevision)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// The offlinegraph ledger's correction/retraction/compensating-rollback
// semantics apply to this one persisted, grounded candidate. Labels are human
// annotations; they never change the evidence-derived concept_overlap type.
func planGroundedReview(status model.MentalLinkStatus, oldLabel *string, response model.MentalModelLinkResponse) (model.MentalLinkStatus, error) {
	action := string(response.Action)
	label := strings.TrimSpace(response.Label)
	if len(label) > 160 || len(response.Reason) > 500 {
		return "", ErrReviewInvalid
	}
	switch action {
	case "confirmed":
		if status != model.MentalLinkCandidate || label != "" {
			return "", ErrReviewInvalid
		}
		return model.MentalLinkConfirmed, nil
	case "rejected":
		if status != model.MentalLinkCandidate || strings.TrimSpace(response.Reason) == "" {
			return "", ErrReviewInvalid
		}
		return model.MentalLinkRejected, nil
	case "relabeled":
		if (status != model.MentalLinkCandidate && status != model.MentalLinkConfirmed && status != model.MentalLinkRelabeled) || label == "" || strings.EqualFold(label, "claim_extension") || strings.EqualFold(label, "assumption_conflict") || strings.EqualFold(label, "question_resolution") {
			return "", ErrReviewInvalid
		}
		if oldLabel != nil && *oldLabel == label && status == model.MentalLinkRelabeled {
			return "", ErrReviewInvalid
		}
		return model.MentalLinkRelabeled, nil
	case "retracted":
		if (status != model.MentalLinkConfirmed && status != model.MentalLinkRelabeled) || strings.TrimSpace(response.Reason) == "" {
			return "", ErrReviewInvalid
		}
		return model.MentalLinkArchived, nil
	case "rolled_back":
		if response.TargetRevision == nil || strings.TrimSpace(response.Reason) == "" {
			return "", ErrReviewInvalid
		}
		return status, nil // actual target state is resolved from the immutable event chain in the transaction
	default:
		return "", ErrReviewInvalid
	}
}

// PreviewMentalModelLink has no write side effects; ListMentalModelLinks applies
// the live owner, two-sided witness, and document-snapshot checks.
func (s *Store) PreviewMentalModelLink(ctx context.Context, owner, id string) (*model.MentalLinkReviewPreview, error) {
	links, err := s.ListMentalModelLinks(ctx, owner, "")
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		if link.ID == id {
			preview := &model.MentalLinkReviewPreview{MentalModelLink: link, History: []model.MentalLinkReviewEvent{}}
			rows, err := s.pool.Query(ctx, `SELECT revision, action, before_status, after_status, before_label, after_label, reason, target_revision, occurred_at FROM mental_link_review_events WHERE user_id=$1 AND link_id=$2 ORDER BY revision`, owner, id)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var event model.MentalLinkReviewEvent
				if err := rows.Scan(&event.Revision, &event.Action, &event.BeforeStatus, &event.AfterStatus, &event.BeforeLabel, &event.AfterLabel, &event.Reason, &event.TargetRevision, &event.OccurredAt); err != nil {
					return nil, err
				}
				preview.History = append(preview.History, event)
			}
			return preview, rows.Err()
		}
	}
	return nil, ErrMentalModelLinkNotFound
}
