package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/model"
)

func (s *Store) listMessageFeedback(ctx context.Context, userID, messageID string) ([]model.ChatFeedback, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, message_id, action, correction_text, created_at
		FROM chat_message_feedback WHERE user_id = $1 AND message_id = $2
		ORDER BY created_at`, userID, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	feedback := []model.ChatFeedback{}
	for rows.Next() {
		var item model.ChatFeedback
		if err := rows.Scan(&item.ID, &item.MessageID, &item.Action, &item.CorrectionText, &item.CreatedAt); err != nil {
			return nil, err
		}
		feedback = append(feedback, item)
	}
	return feedback, rows.Err()
}

// SoftDeleteChatThread hides a conversation while retaining its immutable
// evidence and graph provenance.
func (s *Store) SoftDeleteChatThread(ctx context.Context, userID, threadID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `
		UPDATE chat_threads SET deleted_at = now(), updated_at = now(), retention_policy = 'retain_evidence'
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, threadID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO learning_events (user_id, event_type, payload, source, idempotency_key)
		VALUES ($1, 'chat_thread_deleted', jsonb_build_object('thread_id', $2::text, 'retention_policy', 'retain_evidence'),
		        'chat', $3)
		ON CONFLICT (user_id, idempotency_key) DO NOTHING`, userID, threadID, "chat-thread-deleted:"+threadID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordCitationOpen(ctx context.Context, userID, citationID string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var messageID, chunkID, documentID string
	err = tx.QueryRow(ctx, `
		SELECT mc.message_id, mc.chunk_id, c.document_id
		FROM message_citations mc
		JOIN chat_messages m ON m.id = mc.message_id
		JOIN chunks c ON c.id = mc.chunk_id
		WHERE mc.id = $1 AND m.user_id = $2`, citationID, userID).Scan(&messageID, &chunkID, &documentID)
	if err != nil {
		return false, err
	}
	var interactionID string
	err = tx.QueryRow(ctx, `
		INSERT INTO citation_interactions (user_id, citation_id, event_type)
		VALUES ($1, $2, 'opened')
		ON CONFLICT (user_id, citation_id, event_type) DO NOTHING
		RETURNING id`, userID, citationID).Scan(&interactionID)
	if err == pgx.ErrNoRows {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `
		UPDATE message_citations SET open_count = open_count + 1, last_opened_at = now()
		WHERE id = $1`, citationID); err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT concept_id FROM chat_concept_evidence
		WHERE message_id = $1 AND active ORDER BY concept_id`, messageID)
	if err != nil {
		return false, err
	}
	var conceptIDs []string
	for rows.Next() {
		var conceptID string
		if err := rows.Scan(&conceptID); err != nil {
			rows.Close()
			return false, err
		}
		conceptIDs = append(conceptIDs, conceptID)
	}
	rows.Close()
	for _, conceptID := range conceptIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO learning_events (
				user_id, event_type, chat_message_id, document_id, chunk_id, concept_id,
				payload, source, idempotency_key
			) VALUES ($1, 'citation_opened', $2, $3, $4, $5,
			          jsonb_build_object('citation_id', $6::text), 'chat', $7)
			ON CONFLICT (user_id, idempotency_key) DO NOTHING`, userID, messageID, documentID,
			chunkID, conceptID, citationID, fmt.Sprintf("citation-open:%s:%s", citationID, conceptID))
		if err != nil {
			return false, err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO chat_learner_projection (
				user_id, concept_id, retrieval_count, interest_score, updated_at
			) VALUES ($1, $2, 1, 0.10, now())
			ON CONFLICT (user_id, concept_id) DO UPDATE SET
				retrieval_count = chat_learner_projection.retrieval_count + 1,
				interest_score = LEAST(1, chat_learner_projection.interest_score + 0.10),
				updated_at = now()`, userID, conceptID)
		if err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}

func (s *Store) RecordChatFeedback(ctx context.Context, userID, messageID string, request model.ChatFeedbackRequest) (*model.ChatFeedback, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var threadID, role string
	err = tx.QueryRow(ctx, `
		SELECT m.thread_id, m.role FROM chat_messages m
		JOIN chat_threads t ON t.id = m.thread_id
		WHERE m.id = $1 AND m.user_id = $2 AND t.deleted_at IS NULL`, messageID, userID).Scan(&threadID, &role)
	if err != nil {
		return nil, err
	}
	if role != "assistant" {
		return nil, fmt.Errorf("feedback is only supported for assistant messages")
	}
	feedback := &model.ChatFeedback{}
	err = tx.QueryRow(ctx, `
		INSERT INTO chat_message_feedback (user_id, message_id, action, correction_text)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, message_id, action) DO NOTHING
		RETURNING id, message_id, action, correction_text, created_at`, userID, messageID,
		request.Action, request.CorrectionText).Scan(&feedback.ID, &feedback.MessageID, &feedback.Action,
		&feedback.CorrectionText, &feedback.CreatedAt)
	inserted := err == nil
	if err == pgx.ErrNoRows {
		if request.CorrectionText != "" {
			err = tx.QueryRow(ctx, `
				UPDATE chat_message_feedback SET correction_text = $4
				WHERE user_id = $1 AND message_id = $2 AND action = $3
				RETURNING id, message_id, action, correction_text, created_at`,
				userID, messageID, request.Action, request.CorrectionText).Scan(&feedback.ID, &feedback.MessageID,
				&feedback.Action, &feedback.CorrectionText, &feedback.CreatedAt)
		} else {
			err = tx.QueryRow(ctx, `
				SELECT id, message_id, action, correction_text, created_at
				FROM chat_message_feedback WHERE user_id = $1 AND message_id = $2 AND action = $3`,
				userID, messageID, request.Action).Scan(&feedback.ID, &feedback.MessageID, &feedback.Action,
				&feedback.CorrectionText, &feedback.CreatedAt)
		}
	}
	if err != nil {
		return nil, err
	}
	if !inserted {
		return feedback, tx.Commit(ctx)
	}

	conceptRows, err := tx.Query(ctx, `
		SELECT DISTINCT concept_id FROM chat_concept_evidence
		WHERE message_id = $1 AND active ORDER BY concept_id`, messageID)
	if err != nil {
		return nil, err
	}
	var conceptIDs []string
	for conceptRows.Next() {
		var conceptID string
		if err := conceptRows.Scan(&conceptID); err != nil {
			conceptRows.Close()
			return nil, err
		}
		conceptIDs = append(conceptIDs, conceptID)
	}
	conceptRows.Close()

	eventType := "chat_feedback_" + request.Action
	for _, conceptID := range conceptIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO learning_events (user_id, event_type, chat_message_id, concept_id, payload, source, idempotency_key)
			VALUES ($1, $2, $3, $4, jsonb_build_object('feedback_id', $5::text), 'chat', $6)
			ON CONFLICT (user_id, idempotency_key) DO NOTHING`, userID, eventType, messageID,
			conceptID, feedback.ID, fmt.Sprintf("feedback:%s:%s", feedback.ID, conceptID))
		if err != nil {
			return nil, err
		}
		if request.Action == "helpful" || request.Action == "unhelpful" {
			success, failure, delta := 0, 0, 0.08
			if request.Action == "helpful" {
				success = 1
			} else {
				failure, delta = 1, -0.05
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO chat_learner_projection (
					user_id, concept_id, success_count, failure_count, interest_score, updated_at
				) VALUES ($1, $2, $3, $4, GREATEST(0, $5::real), now())
				ON CONFLICT (user_id, concept_id) DO UPDATE SET
					success_count = chat_learner_projection.success_count + $3,
					failure_count = chat_learner_projection.failure_count + $4,
					interest_score = LEAST(1, GREATEST(0, chat_learner_projection.interest_score + $5::real)),
					updated_at = now()`, userID, conceptID, success, failure, delta)
			if err != nil {
				return nil, err
			}
		}
	}

	if request.Action == "correction" {
		if _, err = tx.Exec(ctx, `UPDATE chat_messages SET status = 'superseded' WHERE id = $1`, messageID); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO chat_messages (
				thread_id, user_id, role, content, status, model_version, supersedes_message_id
			) VALUES ($1, $2, 'system', $3, 'complete', 'user-correction-v1', $4)`,
			threadID, userID, "Correction recorded: "+request.CorrectionText, messageID); err != nil {
			return nil, err
		}
		edgeRows, err := tx.Query(ctx, `
			SELECT DISTINCT edge_id FROM adaptive_edge_evidence
			WHERE message_id = $1 AND active ORDER BY edge_id`, messageID)
		if err != nil {
			return nil, err
		}
		var edgeIDs []string
		for edgeRows.Next() {
			var edgeID string
			if err := edgeRows.Scan(&edgeID); err != nil {
				edgeRows.Close()
				return nil, err
			}
			edgeIDs = append(edgeIDs, edgeID)
		}
		edgeRows.Close()
		if _, err = tx.Exec(ctx, `
			UPDATE adaptive_edge_evidence SET active = false, superseded_at = now()
			WHERE message_id = $1 AND active`, messageID); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE chat_concept_evidence SET active = false, superseded_at = now()
			WHERE message_id = $1 AND active`, messageID); err != nil {
			return nil, err
		}
		for _, conceptID := range conceptIDs {
			if _, err = tx.Exec(ctx, `
				UPDATE chat_learner_projection SET
					exposure_count = GREATEST(0, exposure_count - 1),
					interest_score = GREATEST(0, interest_score - 0.05), updated_at = now()
				WHERE user_id = $1 AND concept_id = $2`, userID, conceptID); err != nil {
				return nil, err
			}
		}
		for _, edgeID := range edgeIDs {
			if _, err = tx.Exec(ctx, `
				INSERT INTO graph_edge_actions (user_id, edge_id, action, reason, source_message_id)
				VALUES ($1, $2, 'weakened', 'User correction superseded chat evidence', $3)`,
				userID, edgeID, messageID); err != nil {
				return nil, err
			}
			if err = recalculateAdaptiveEdge(ctx, tx, edgeID, time.Now().UTC()); err != nil {
				return nil, err
			}
		}
	}
	return feedback, tx.Commit(ctx)
}

func recalculateAdaptiveEdge(ctx context.Context, tx pgx.Tx, edgeID string, asOf time.Time) error {
	var createdVia, currentState string
	var baseConfidence float64
	err := tx.QueryRow(ctx, `SELECT created_via, state, base_confidence FROM concept_edges WHERE id = $1`, edgeID).
		Scan(&createdVia, &currentState, &baseConfidence)
	if err != nil {
		return err
	}
	var messageCount, documentCount int
	var lastObserved *time.Time
	err = tx.QueryRow(ctx, `
		SELECT count(*)::int,
		       (SELECT count(DISTINCT document_id)::int FROM (
		          SELECT source_document_id AS document_id FROM adaptive_edge_evidence WHERE edge_id = $1 AND active
		          UNION ALL
		          SELECT target_document_id FROM adaptive_edge_evidence WHERE edge_id = $1 AND active
		       ) documents),
		       max(created_at)
		FROM adaptive_edge_evidence WHERE edge_id = $1 AND active`, edgeID).
		Scan(&messageCount, &documentCount, &lastObserved)
	if err != nil {
		return err
	}
	state, evidenceConfidence, validTo := projectedEdgeLifecycle(createdVia, currentState, messageCount, documentCount, lastObserved, asOf)
	confidence := math.Max(baseConfidence, evidenceConfidence)
	_, err = tx.Exec(ctx, `
		UPDATE concept_edges SET support_count = $2, document_count = $3,
		       evidence_confidence = $4, confidence = $5, state = $6,
		       valid_to = $7, last_adapted_at = $8
		WHERE id = $1`, edgeID, messageCount, documentCount, evidenceConfidence, confidence, state, validTo, asOf)
	return err
}

func projectedEdgeLifecycle(createdVia, currentState string, messageCount, documentCount int, lastObserved *time.Time, asOf time.Time) (string, float64, *time.Time) {
	if currentState == "confirmed" || currentState == "rejected" || currentState == "superseded" {
		return currentState, 0, nil
	}
	if messageCount == 0 {
		if createdVia == "deterministic_chat" {
			closed := asOf
			return "archived", 0, &closed
		}
		return currentState, 0, nil
	}
	evidenceConfidence := adaptiveEdgeConfidence(messageCount, documentCount)
	ageDays := 0.0
	if lastObserved != nil && asOf.After(*lastObserved) {
		ageDays = asOf.Sub(*lastObserved).Hours() / 24
	}
	evidenceConfidence *= math.Pow(0.5, ageDays/30)
	state := adaptiveEdgeState(messageCount, documentCount)
	if evidenceConfidence < 0.45 {
		state = "candidate"
	}
	if createdVia == "deterministic_chat" && ageDays >= 90 {
		closed := asOf
		return "archived", evidenceConfidence, &closed
	}
	return state, evidenceConfidence, nil
}

func (s *Store) RespondToConceptEdge(ctx context.Context, userID, edgeID string, request model.EdgeActionRequest) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	state := request.Action
	if request.Action == "confirm" {
		state = "confirmed"
	}
	if request.Action == "reject" {
		state = "rejected"
	}
	result, err := tx.Exec(ctx, `
		UPDATE concept_edges SET state = $3,
		       confirmed_at = CASE WHEN $3 = 'confirmed' THEN now() ELSE confirmed_at END,
		       base_confidence = CASE WHEN $3 = 'confirmed' THEN GREATEST(base_confidence, 0.95) ELSE base_confidence END,
		       confidence = CASE WHEN $3 = 'confirmed' THEN GREATEST(confidence, 0.95) ELSE confidence END,
		       valid_to = CASE WHEN $3 = 'rejected' THEN now() ELSE NULL END
		WHERE id = $1 AND user_id = $2`, edgeID, userID, state)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO graph_edge_actions (user_id, edge_id, action, reason)
		VALUES ($1, $2, $3, $4)`, userID, edgeID, state, request.Reason)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO learning_events (user_id, event_type, concept_edge_id, payload, source, idempotency_key)
		VALUES ($1, $2, $3, jsonb_build_object('reason', $4::text), 'user', $5)`, userID,
		"graph_edge_"+state, edgeID, request.Reason, fmt.Sprintf("edge-action:%s:%s:%d", edgeID, state, time.Now().UnixNano()))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordLearnerSignal(ctx context.Context, userID string, request model.LearnerSignalRequest) error {
	var success, failure int
	switch request.Signal {
	case "quiz_success":
		success = 1
	case "quiz_failure":
		failure = 1
	default:
		return fmt.Errorf("unsupported learner signal")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM concepts WHERE id = $1 AND user_id = $2)`, request.ConceptID, userID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return pgx.ErrNoRows
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO learning_events (user_id, event_type, concept_id, payload, source, idempotency_key)
		VALUES ($1, $2, $3, '{}', 'quiz', $4)
		ON CONFLICT (user_id, idempotency_key) DO NOTHING`, userID, request.Signal, request.ConceptID,
		"learner-signal:"+request.IdempotencyID)
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		delta := 0.08
		if failure == 1 {
			delta = -0.05
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO chat_learner_projection (
				user_id, concept_id, success_count, failure_count, interest_score, updated_at
			) VALUES ($1, $2, $3, $4, GREATEST(0, $5::real), now())
			ON CONFLICT (user_id, concept_id) DO UPDATE SET
				success_count = chat_learner_projection.success_count + $3,
				failure_count = chat_learner_projection.failure_count + $4,
				interest_score = LEAST(1, GREATEST(0, chat_learner_projection.interest_score + $5::real)),
				updated_at = now()`, userID, request.ConceptID, success, failure, delta)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type replayEdge struct {
	id, createdVia, currentState, expectedState          string
	baseConfidence, currentConfidence, expectedEvidence  float64
	currentSupport, currentDocuments, support, documents int
	lastObserved                                         *time.Time
	validTo                                              *time.Time
}

type replayLearner struct {
	conceptID                             string
	exposure, retrieval, success, failure int
	interest                              float64
	lastExposed                           *time.Time
}

func (s *Store) ReplayAdaptiveGraph(ctx context.Context, userID string, apply bool, asOf time.Time) (*model.ReplayReport, error) {
	asOf = asOf.UTC().Truncate(time.Second)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT e.id, e.created_via, e.state, e.base_confidence, e.confidence,
		       e.support_count, e.document_count,
		       (SELECT count(*)::int FROM adaptive_edge_evidence a WHERE a.edge_id = e.id AND a.active),
		       (SELECT count(DISTINCT document_id)::int FROM (
		          SELECT source_document_id AS document_id FROM adaptive_edge_evidence a WHERE a.edge_id = e.id AND a.active
		          UNION ALL
		          SELECT target_document_id FROM adaptive_edge_evidence a WHERE a.edge_id = e.id AND a.active
		       ) documents),
		       (SELECT max(created_at) FROM adaptive_edge_evidence a WHERE a.edge_id = e.id AND a.active)
		FROM concept_edges e
		WHERE e.user_id = $1
		  AND EXISTS (SELECT 1 FROM adaptive_edge_evidence a WHERE a.edge_id = e.id)
		ORDER BY e.id`, userID)
	if err != nil {
		return nil, err
	}
	var edges []replayEdge
	for rows.Next() {
		var edge replayEdge
		if err := rows.Scan(&edge.id, &edge.createdVia, &edge.currentState, &edge.baseConfidence,
			&edge.currentConfidence, &edge.currentSupport, &edge.currentDocuments, &edge.support,
			&edge.documents, &edge.lastObserved); err != nil {
			rows.Close()
			return nil, err
		}
		edge.expectedState, edge.expectedEvidence, edge.validTo = projectedEdgeLifecycle(
			edge.createdVia, edge.currentState, edge.support, edge.documents, edge.lastObserved, asOf)
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	learnerRows, err := tx.Query(ctx, `
		WITH exposures AS (
			SELECT cce.concept_id, count(*)::int AS exposure_count, max(cce.created_at) AS last_exposed_at
			FROM chat_concept_evidence cce
			JOIN chat_messages m ON m.id = cce.message_id
			WHERE m.user_id = $1 AND cce.active GROUP BY cce.concept_id
		), signals AS (
			SELECT concept_id,
			 count(*) FILTER (WHERE event_type = 'citation_opened')::int AS retrieval_count,
			 count(*) FILTER (WHERE event_type IN ('chat_feedback_helpful', 'quiz_success'))::int AS success_count,
			 count(*) FILTER (WHERE event_type IN ('chat_feedback_unhelpful', 'quiz_failure'))::int AS failure_count
			FROM learning_events WHERE user_id = $1 AND concept_id IS NOT NULL GROUP BY concept_id
		), ids AS (
			SELECT concept_id FROM exposures UNION SELECT concept_id FROM signals
		)
		SELECT ids.concept_id, COALESCE(e.exposure_count, 0), COALESCE(s.retrieval_count, 0),
		       COALESCE(s.success_count, 0), COALESCE(s.failure_count, 0), e.last_exposed_at
		FROM ids LEFT JOIN exposures e USING (concept_id) LEFT JOIN signals s USING (concept_id)
		ORDER BY ids.concept_id`, userID)
	if err != nil {
		return nil, err
	}
	var learners []replayLearner
	for learnerRows.Next() {
		var learner replayLearner
		if err := learnerRows.Scan(&learner.conceptID, &learner.exposure, &learner.retrieval,
			&learner.success, &learner.failure, &learner.lastExposed); err != nil {
			learnerRows.Close()
			return nil, err
		}
		learner.interest = math.Min(1, math.Max(0, float64(learner.exposure)*0.05+
			float64(learner.retrieval)*0.10+float64(learner.success)*0.08-float64(learner.failure)*0.05))
		learners = append(learners, learner)
	}
	learnerRows.Close()

	differences := 0
	for _, edge := range edges {
		expectedConfidence := math.Max(edge.baseConfidence, edge.expectedEvidence)
		if edge.currentState != edge.expectedState || edge.currentSupport != edge.support ||
			edge.currentDocuments != edge.documents || math.Abs(edge.currentConfidence-expectedConfidence) > 0.0001 {
			differences++
		}
		if apply {
			_, err = tx.Exec(ctx, `
				UPDATE concept_edges SET state = $2, support_count = $3, document_count = $4,
				       evidence_confidence = $5, confidence = GREATEST(base_confidence, $5),
				       valid_to = $6, last_adapted_at = $7
				WHERE id = $1`, edge.id, edge.expectedState, edge.support, edge.documents,
				edge.expectedEvidence, edge.validTo, asOf)
			if err != nil {
				return nil, err
			}
		}
	}
	currentLearners := map[string]replayLearner{}
	currentRows, err := tx.Query(ctx, `
		SELECT concept_id, exposure_count, retrieval_count, success_count, failure_count,
		       interest_score, last_exposed_at
		FROM chat_learner_projection WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	for currentRows.Next() {
		var learner replayLearner
		if err := currentRows.Scan(&learner.conceptID, &learner.exposure, &learner.retrieval,
			&learner.success, &learner.failure, &learner.interest, &learner.lastExposed); err != nil {
			currentRows.Close()
			return nil, err
		}
		currentLearners[learner.conceptID] = learner
	}
	currentRows.Close()
	for _, expected := range learners {
		current, ok := currentLearners[expected.conceptID]
		if !ok || current.exposure != expected.exposure || current.retrieval != expected.retrieval ||
			current.success != expected.success || current.failure != expected.failure ||
			math.Abs(current.interest-expected.interest) > 0.0001 {
			differences++
		}
		delete(currentLearners, expected.conceptID)
	}
	differences += len(currentLearners)

	if apply {
		if _, err = tx.Exec(ctx, `DELETE FROM chat_learner_projection WHERE user_id = $1`, userID); err != nil {
			return nil, err
		}
		for _, learner := range learners {
			_, err = tx.Exec(ctx, `
				INSERT INTO chat_learner_projection (
					user_id, concept_id, exposure_count, retrieval_count, success_count,
					failure_count, interest_score, last_exposed_at, reducer_version, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'learner-projection-v1', $9)`,
				userID, learner.conceptID, learner.exposure, learner.retrieval, learner.success,
				learner.failure, learner.interest, learner.lastExposed, asOf)
			if err != nil {
				return nil, err
			}
		}
	}

	parts := make([]string, 0, len(edges)+len(learners))
	for _, edge := range edges {
		parts = append(parts, fmt.Sprintf("e:%s:%s:%d:%d:%.6f", edge.id, edge.expectedState,
			edge.support, edge.documents, edge.expectedEvidence))
	}
	for _, learner := range learners {
		parts = append(parts, fmt.Sprintf("l:%s:%d:%d:%d:%d:%.6f", learner.conceptID,
			learner.exposure, learner.retrieval, learner.success, learner.failure, learner.interest))
	}
	sort.Strings(parts)
	hashBytes := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	report := &model.ReplayReport{
		ReducerVersion: "adaptive-replay-v1", AsOf: asOf, Applied: apply,
		Differences: differences, ProjectionHash: hex.EncodeToString(hashBytes[:]),
		EdgeCount: len(edges), LearnerCount: len(learners), Equivalent: differences == 0,
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO graph_replay_runs (
			user_id, reducer_version, as_of, applied, differences, projection_hash
		) VALUES ($1, $2, $3, $4, $5, $6)`, userID, report.ReducerVersion, report.AsOf,
		report.Applied, report.Differences, report.ProjectionHash)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Store) GetMetricsSummary(ctx context.Context, userID string) (*model.MetricsSummary, error) {
	summary := &model.MetricsSummary{}
	err := s.pool.QueryRow(ctx, `
		SELECT count(*)::int,
		       COALESCE(avg((values->>'total_ms')::double precision), 0),
		       COALESCE(avg((values->>'retrieval_ms')::double precision), 0),
		       COALESCE(avg((values->>'citation_count')::double precision), 0)
		FROM evaluation_metrics WHERE user_id = $1 AND metric_type = 'chat_turn'`, userID).
		Scan(&summary.ChatTurns, &summary.AverageTotalMS, &summary.AverageRetrievalMS, &summary.AverageCitations)
	if err != nil {
		return nil, err
	}
	err = s.pool.QueryRow(ctx, `
		SELECT
		 (SELECT count(*)::int FROM citation_interactions WHERE user_id = $1),
		 (SELECT count(*)::int FROM chat_message_feedback WHERE user_id = $1 AND action = 'helpful'),
		 (SELECT count(*)::int FROM chat_message_feedback WHERE user_id = $1 AND action = 'unhelpful'),
		 (SELECT count(*)::int FROM chat_message_feedback WHERE user_id = $1 AND action = 'correction'),
		 (SELECT count(*)::int FROM graph_edge_actions WHERE user_id = $1 AND action = 'confirmed'),
		 (SELECT count(*)::int FROM graph_edge_actions WHERE user_id = $1 AND action = 'rejected')`, userID).
		Scan(&summary.CitationOpens, &summary.HelpfulAnswers, &summary.UnhelpfulAnswers,
			&summary.Corrections, &summary.ConfirmedEdges, &summary.RejectedEdges)
	return summary, err
}
