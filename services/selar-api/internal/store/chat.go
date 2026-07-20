package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/model"
)

// CreateChatThread starts a user-owned conversation.
func (s *Store) CreateChatThread(ctx context.Context, userID, title string) (*model.ChatThread, error) {
	thread := &model.ChatThread{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO chat_threads (user_id, title) VALUES ($1, $2)
		 RETURNING id, user_id, title, created_at, updated_at`, userID, title).Scan(
		&thread.ID, &thread.UserID, &thread.Title, &thread.CreatedAt, &thread.UpdatedAt,
	)
	return thread, err
}

// ListChatThreads returns the most recently active conversations first.
func (s *Store) ListChatThreads(ctx context.Context, userID string) ([]model.ChatThread, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, title, created_at, updated_at
		 FROM chat_threads WHERE user_id = $1 AND deleted_at IS NULL ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	threads := []model.ChatThread{}
	for rows.Next() {
		var thread model.ChatThread
		if err := rows.Scan(&thread.ID, &thread.UserID, &thread.Title, &thread.CreatedAt, &thread.UpdatedAt); err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

// CreateChatMessage persists a user episode only when the thread belongs to the user.
func (s *Store) CreateChatMessage(ctx context.Context, userID, threadID, role, content, status, modelVersion string) (*model.ChatMessage, error) {
	message := &model.ChatMessage{Citations: []model.ChatCitation{}}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO chat_messages (thread_id, user_id, role, content, status, model_version)
		 SELECT t.id, $1, $3, $4, $5, $6 FROM chat_threads t
		 WHERE t.id = $2 AND t.user_id = $1 AND t.deleted_at IS NULL
		 RETURNING id, thread_id, user_id, role, content, status, model_version, created_at,
		           COALESCE(supersedes_message_id::text, '')`,
		userID, threadID, role, content, status, modelVersion).Scan(
		&message.ID, &message.ThreadID, &message.UserID, &message.Role, &message.Content,
		&message.Status, &message.ModelVersion, &message.CreatedAt, &message.SupersedesMessageID,
	)
	return message, err
}

// ListChatMessages returns a thread with citations while enforcing ownership.
func (s *Store) ListChatMessages(ctx context.Context, userID, threadID string) ([]model.ChatMessage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.thread_id, m.user_id, m.role, m.content, m.status, m.model_version, m.created_at,
		        COALESCE(m.supersedes_message_id::text, '')
		 FROM chat_messages m JOIN chat_threads t ON t.id = m.thread_id
		 WHERE t.user_id = $1 AND t.deleted_at IS NULL AND m.thread_id = $2 ORDER BY m.created_at`, userID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []model.ChatMessage{}
	for rows.Next() {
		var message model.ChatMessage
		message.Citations = []model.ChatCitation{}
		if err := rows.Scan(&message.ID, &message.ThreadID, &message.UserID, &message.Role,
			&message.Content, &message.Status, &message.ModelVersion, &message.CreatedAt,
			&message.SupersedesMessageID); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range messages {
		citations, err := s.listMessageCitations(ctx, messages[index].ID)
		if err != nil {
			return nil, err
		}
		messages[index].Citations = citations
		graphUpdate, err := s.loadChatGraphUpdate(ctx, messages[index].ID)
		if err != nil {
			return nil, err
		}
		messages[index].GraphUpdate = graphUpdate
		feedback, err := s.listMessageFeedback(ctx, userID, messages[index].ID)
		if err != nil {
			return nil, err
		}
		messages[index].Feedback = feedback
	}
	return messages, nil
}

func (s *Store) listMessageCitations(ctx context.Context, messageID string) ([]model.ChatCitation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT mc.id, mc.message_id, mc.chunk_id, c.document_id, d.title, c.page_start,
		        mc.rank, mc.score, mc.quote, d.source_type, c.locator
		 FROM message_citations mc
		 JOIN chunks c ON c.id = mc.chunk_id
		 JOIN documents d ON d.id = c.document_id
		 WHERE mc.message_id = $1 ORDER BY mc.rank`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	citations := []model.ChatCitation{}
	for rows.Next() {
		var citation model.ChatCitation
		if err := rows.Scan(&citation.ID, &citation.MessageID, &citation.ChunkID,
			&citation.DocumentID, &citation.DocumentTitle, &citation.Page, &citation.Rank,
			&citation.Score, &citation.Quote, &citation.SourceType, &citation.Locator); err != nil {
			return nil, err
		}
		citations = append(citations, citation)
	}
	return citations, rows.Err()
}

// SaveChatAnswer atomically stores the answer, provenance, retrieval trace, and deterministic events.
func (s *Store) SaveChatAnswer(ctx context.Context, userID, threadID, userMessageID, query string, answer model.ChatAnswer) (*model.ChatMessage, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	message := &model.ChatMessage{Citations: []model.ChatCitation{}}
	err = tx.QueryRow(ctx,
		`INSERT INTO chat_messages (thread_id, user_id, role, content, status, model_version)
		 SELECT t.id, $1, 'assistant', $3, 'complete', $4 FROM chat_threads t
		 WHERE t.id = $2 AND t.user_id = $1 AND t.deleted_at IS NULL
		 RETURNING id, thread_id, user_id, role, content, status, model_version, created_at,
		           COALESCE(supersedes_message_id::text, '')`,
		userID, threadID, answer.Answer, answer.ModelVersion).Scan(
		&message.ID, &message.ThreadID, &message.UserID, &message.Role, &message.Content,
		&message.Status, &message.ModelVersion, &message.CreatedAt, &message.SupersedesMessageID)
	if err != nil {
		return nil, err
	}
	for _, citation := range answer.Citations {
		var saved model.ChatCitation
		err = tx.QueryRow(ctx,
			`INSERT INTO message_citations (message_id, chunk_id, rank, score, quote)
			 SELECT $1, c.id, $3, $4, $5 FROM chunks c WHERE c.id = $2 AND c.user_id = $6
			 RETURNING id, message_id, chunk_id, rank, score, quote`,
			message.ID, citation.ChunkID, citation.Rank, citation.Score, citation.Quote, userID).Scan(
			&saved.ID, &saved.MessageID, &saved.ChunkID, &saved.Rank, &saved.Score, &saved.Quote)
		if err != nil {
			return nil, err
		}
		saved.DocumentID = citation.DocumentID
		saved.DocumentTitle = citation.DocumentTitle
		saved.Page = citation.Page
		saved.SourceType = citation.SourceType
		saved.Locator = citation.Locator
		message.Citations = append(message.Citations, saved)
	}
	metrics, err := json.Marshal(map[string]any{
		"embedding_ms": answer.Metrics.EmbeddingMS, "retrieval_ms": answer.Metrics.RetrievalMS,
		"generation_ms": answer.Metrics.GenerationMS, "total_ms": answer.Metrics.TotalMS,
		"candidate_count": answer.Metrics.CandidateCount, "citation_count": len(message.Citations),
		"model_calls": answer.Metrics.ModelCalls,
	})
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO evaluation_metrics (user_id, chat_message_id, metric_type, values)
		 VALUES ($1, $2, 'chat_turn', $3)`, userID, message.ID, metrics)
	if err != nil {
		return nil, err
	}

	graphUpdate, err := reduceChatGraph(ctx, tx, userID, message.ID, query)
	if err != nil {
		return nil, err
	}
	message.GraphUpdate = graphUpdate

	candidates, err := json.Marshal(answer.Candidates)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO retrieval_traces (user_id, thread_id, user_message_id, assistant_message_id, query, ranking_policy, candidates)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`, userID, threadID, userMessageID,
		message.ID, query, answer.RankingPolicy, candidates)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO learning_events (user_id, event_type, chat_message_id, payload, source, idempotency_key)
		 VALUES ($1, 'chat_graph_reduced', $2,
		         jsonb_build_object('concepts_created', $3::int, 'concepts_reinforced', $4::int,
		                            'links_observed', $5::int, 'links_promoted', $6::int,
		                            'reducer_version', $7::text),
		         'deterministic_reducer', $8)
		 ON CONFLICT (user_id, idempotency_key) DO NOTHING`, userID, message.ID,
		graphUpdate.ConceptsCreated, graphUpdate.ConceptsReinforced, graphUpdate.LinksObserved, graphUpdate.LinksPromoted,
		graphUpdate.ReducerVersion, fmt.Sprintf("chat-graph-reduced:%s", message.ID))
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO learning_events (user_id, event_type, chat_message_id, payload, source, idempotency_key)
		 VALUES ($1, 'chat_question', $2, jsonb_build_object('thread_id', $3::text), 'chat', $4),
		        ($1, 'chat_answer', $5, jsonb_build_object('thread_id', $3::text, 'citation_count', $6::int), 'chat', $7)
		 ON CONFLICT (user_id, idempotency_key) DO NOTHING`, userID, userMessageID, threadID,
		fmt.Sprintf("chat-question:%s", userMessageID), message.ID, len(message.Citations),
		fmt.Sprintf("chat-answer:%s", message.ID))
	if err != nil {
		return nil, err
	}

	title := query
	if len(title) > 72 {
		title = title[:72]
	}
	_, err = tx.Exec(ctx,
		`UPDATE chat_threads SET updated_at = $1,
		 title = CASE WHEN title = 'New conversation' THEN $2 ELSE title END
		 WHERE id = $3 AND user_id = $4`, time.Now(), title, threadID, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return message, nil
}

type chatConceptEvidence struct {
	conceptID         string
	chunkID           string
	documentID        string
	rank              int
	score             float32
	bindingMethod     string
	bindingConfidence float32
}

var groundedQuestionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*(?:what|who)\s+(?:is|are|was|were)\s+(.+?)\s*[?.!]*\s*$`),
	regexp.MustCompile(`(?i)^\s*(?:define|explain|describe)\s+(.+?)\s*[?.!]*\s*$`),
	regexp.MustCompile(`(?i)^\s*tell\s+me\s+about\s+(.+?)\s*[?.!]*\s*$`),
}

func conceptTermFromQuery(query string) (string, bool) {
	for _, pattern := range groundedQuestionPatterns {
		matches := pattern.FindStringSubmatch(query)
		if len(matches) != 2 {
			continue
		}
		term := strings.Trim(strings.TrimSpace(matches[1]), `"'“”‘’`)
		words := strings.Fields(term)
		normalized := normalizeConceptTerm(term)
		if len(words) > 8 || len(normalized) < 3 || len(normalized) > 80 {
			return "", false
		}
		return term, true
	}
	return "", false
}

func normalizeConceptTerm(value string) string {
	var normalized strings.Builder
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			normalized.WriteRune(char)
		}
	}
	return normalized.String()
}

func groundedSurface(content, term string) (string, bool) {
	normalized := normalizeConceptTerm(term)
	if normalized == "" {
		return "", false
	}
	var pattern strings.Builder
	pattern.WriteString(`(?i)(?:^|[^\pL\pN])(`)
	for index, char := range []rune(normalized) {
		if index > 0 {
			pattern.WriteString(`[^\pL\pN]*`)
		}
		pattern.WriteString(regexp.QuoteMeta(string(char)))
	}
	pattern.WriteString(`)(?:$|[^\pL\pN])`)
	matches := regexp.MustCompile(pattern.String()).FindStringSubmatch(content)
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

func groundedConceptDescription(content, surface string) string {
	for _, sentence := range regexp.MustCompile(`[\r\n]+|[.!?]\s+`).Split(content, -1) {
		if strings.Contains(normalizeConceptTerm(sentence), normalizeConceptTerm(surface)) {
			sentence = strings.TrimSpace(sentence)
			if len(sentence) > 500 {
				return sentence[:500] + "…"
			}
			return sentence
		}
	}
	return "Candidate concept discovered from cited library evidence."
}

type groundedCitationChunk struct {
	chunkID    string
	documentID string
	rank       int
	score      float32
	content    string
	surface    string
}

// discoverGroundedChatConcepts creates at most one candidate concept from a
// question, and only when the term is present in a cited source chunk. Neither
// generated answer text nor user feedback comments are treated as factual
// graph evidence.
func discoverGroundedChatConcepts(ctx context.Context, tx pgx.Tx, userID, messageID, query string) (map[string]bool, error) {
	created := map[string]bool{}
	term, ok := conceptTermFromQuery(query)
	if !ok {
		return created, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT mc.chunk_id, c.document_id, mc.rank, mc.score, c.content
		FROM message_citations mc
		JOIN chunks c ON c.id = mc.chunk_id AND c.user_id = $2
		WHERE mc.message_id = $1 ORDER BY mc.rank, mc.chunk_id`, messageID, userID)
	if err != nil {
		return nil, err
	}
	var matches []groundedCitationChunk
	for rows.Next() {
		var item groundedCitationChunk
		if err := rows.Scan(&item.chunkID, &item.documentID, &item.rank, &item.score, &item.content); err != nil {
			rows.Close()
			return nil, err
		}
		if surface, found := groundedSurface(item.content, term); found {
			item.surface = surface
			matches = append(matches, item)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(matches) == 0 {
		return created, nil
	}

	normalized := normalizeConceptTerm(term)
	var conceptID, conceptState string
	err = tx.QueryRow(ctx, `
		SELECT id, state FROM concepts
		WHERE user_id = $1
		  AND regexp_replace(lower(name), '[^[:alnum:]]', '', 'g') = $2
		ORDER BY created_at LIMIT 1`, userID, normalized).Scan(&conceptID, &conceptState)
	newConcept := err == pgx.ErrNoRows
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	if !newConcept && (conceptState == "rejected" || conceptState == "archived") {
		return created, nil
	}
	if newConcept {
		first := matches[0]
		description := groundedConceptDescription(first.content, first.surface)
		err = tx.QueryRow(ctx, `
			INSERT INTO concepts (
				user_id, name, description, embedding, state, model_version, prompt_version
			)
			SELECT $1, $2, $3, c.embedding, 'candidate',
			       'deterministic-grounded-chat', 'grounded-chat-concept-v1'
			FROM chunks c WHERE c.id = $4 AND c.user_id = $1
			RETURNING id`, userID, first.surface, description, first.chunkID).Scan(&conceptID)
		if err != nil {
			return nil, err
		}
		created[conceptID] = true
	}

	for _, item := range matches {
		if _, err = tx.Exec(ctx, `
			INSERT INTO chunk_concepts (chunk_id, concept_id, confidence)
			VALUES ($1, $2, 1.0)
			ON CONFLICT (chunk_id, concept_id) DO UPDATE SET confidence = GREATEST(chunk_concepts.confidence, 1.0)`,
			item.chunkID, conceptID); err != nil {
			return nil, err
		}
	}
	if newConcept {
		_, err = tx.Exec(ctx, `
			INSERT INTO learning_events (
				user_id, event_type, chat_message_id, document_id, chunk_id, concept_id,
				payload, source, idempotency_key
			) VALUES ($1, 'chat_concept_candidate_created', $2, $3, $4, $5,
			          jsonb_build_object('query_term', $6::text, 'binding_method', 'grounded_query_exact'),
			          'deterministic_reducer', $7)
			ON CONFLICT (user_id, idempotency_key) DO NOTHING`, userID, messageID,
			matches[0].documentID, matches[0].chunkID, conceptID, term,
			fmt.Sprintf("grounded-chat-concept:%s:%s", messageID, conceptID))
		if err != nil {
			return nil, err
		}
	}
	return created, nil
}

func (s *Store) loadChatGraphUpdate(ctx context.Context, messageID string) (*model.ChatGraphUpdate, error) {
	update := &model.ChatGraphUpdate{}
	err := s.pool.QueryRow(ctx,
		`SELECT concepts_created, concepts_reinforced, links_observed, links_promoted, reducer_version
		 FROM chat_graph_updates WHERE message_id = $1`, messageID).Scan(
		&update.ConceptsCreated, &update.ConceptsReinforced, &update.LinksObserved,
		&update.LinksPromoted, &update.ReducerVersion)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return update, err
}

// reduceChatGraph converts cited, already-grounded passages into bounded graph
// evidence. It never reads generated answer text and never asks a model to
// choose nodes or edges.
func reduceChatGraph(ctx context.Context, tx pgx.Tx, userID, messageID, query string) (*model.ChatGraphUpdate, error) {
	const reducerVersion = "chat-graph-reducer-v2"
	update := &model.ChatGraphUpdate{ReducerVersion: reducerVersion}

	// Serialize reducers per user so concurrent chat answers cannot create the
	// same undirected edge in opposite directions.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return nil, err
	}

	createdConcepts, err := discoverGroundedChatConcepts(ctx, tx, userID, messageID, query)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		WITH citation_concepts AS (
			SELECT cc.concept_id, mc.chunk_id, c.document_id, mc.rank, mc.score,
			       CASE WHEN concept.prompt_version = 'grounded-chat-concept-v1'
			            THEN 'grounded_query_exact' ELSE 'explicit_chunk_concept' END AS binding_method,
			       cc.confidence AS binding_confidence
			FROM message_citations mc
			JOIN chunks c ON c.id = mc.chunk_id AND c.user_id = $2
			JOIN chunk_concepts cc ON cc.chunk_id = mc.chunk_id
			JOIN concepts concept ON concept.id = cc.concept_id AND concept.user_id = $2
			WHERE mc.message_id = $1 AND concept.state NOT IN ('rejected', 'archived')

			UNION ALL

			SELECT nearest.concept_id, mc.chunk_id, c.document_id, mc.rank, mc.score,
			       'embedding_fallback'::text, nearest.similarity::real
			FROM message_citations mc
			JOIN chunks c ON c.id = mc.chunk_id AND c.user_id = $2
			CROSS JOIN LATERAL (
				SELECT concept.id AS concept_id,
				       1 - (concept.embedding::halfvec(3072) <=> c.embedding::halfvec(3072)) AS similarity
				FROM concepts concept
				WHERE concept.user_id = $2 AND concept.embedding IS NOT NULL
				  AND concept.state NOT IN ('rejected', 'archived')
				ORDER BY concept.embedding::halfvec(3072) <=> c.embedding::halfvec(3072), concept.id
				LIMIT 2
			) nearest
			WHERE mc.message_id = $1 AND c.embedding IS NOT NULL
			  AND nearest.similarity >= 0.65
			  AND NOT EXISTS (SELECT 1 FROM chunk_concepts existing WHERE existing.chunk_id = mc.chunk_id)
		),
		ranked AS (
			SELECT concept_id, chunk_id, document_id, rank, score, binding_method, binding_confidence,
			       row_number() OVER (
			         PARTITION BY concept_id
			         ORDER BY rank, binding_confidence DESC, chunk_id
			       ) AS concept_rank
			FROM citation_concepts
		)
		SELECT concept_id, chunk_id, document_id, rank, score, binding_method, binding_confidence
		FROM ranked WHERE concept_rank = 1
		ORDER BY rank, concept_id
		LIMIT 8`, messageID, userID)
	if err != nil {
		return nil, err
	}
	var evidence []chatConceptEvidence
	for rows.Next() {
		var item chatConceptEvidence
		if err := rows.Scan(&item.conceptID, &item.chunkID, &item.documentID, &item.rank, &item.score,
			&item.bindingMethod, &item.bindingConfidence); err != nil {
			rows.Close()
			return nil, err
		}
		evidence = append(evidence, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for _, item := range evidence {
		result, err := tx.Exec(ctx, `
			INSERT INTO chat_concept_evidence (
				message_id, concept_id, chunk_id, document_id, rank, score, binding_method, binding_confidence
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (message_id, concept_id) DO NOTHING`, messageID, item.conceptID,
			item.chunkID, item.documentID, item.rank, item.score, item.bindingMethod, item.bindingConfidence)
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() == 0 {
			continue
		}
		if createdConcepts[item.conceptID] {
			update.ConceptsCreated++
		} else {
			update.ConceptsReinforced++
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO learner_concept_state (
				user_id, concept_id, last_exposed_at, evidence_count, uncertainty, updated_at
			) VALUES ($1, $2, now(), 1, 0.97, now())
			ON CONFLICT (user_id, concept_id) DO UPDATE SET
				last_exposed_at = now(),
				evidence_count = learner_concept_state.evidence_count + 1,
				uncertainty = GREATEST(0.05, learner_concept_state.uncertainty * 0.97),
				state_version = learner_concept_state.state_version + 1,
				updated_at = now()`, userID, item.conceptID)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO chat_learner_projection (
				user_id, concept_id, exposure_count, interest_score, last_exposed_at, updated_at
			) VALUES ($1, $2, 1, 0.05, now(), now())
			ON CONFLICT (user_id, concept_id) DO UPDATE SET
				exposure_count = chat_learner_projection.exposure_count + 1,
				interest_score = LEAST(1, chat_learner_projection.interest_score + 0.05),
				last_exposed_at = now(), updated_at = now()`, userID, item.conceptID)
		if err != nil {
			return nil, err
		}
	}

	for _, pair := range boundedChatEvidencePairs(evidence, createdConcepts, 3) {
		source := pair[0]
		target := pair[1]
		if target.conceptID < source.conceptID {
			source, target = target, source
		}

		var edgeID, oldState string
		err := tx.QueryRow(ctx, `
				SELECT id, state FROM concept_edges
				WHERE user_id = $1 AND relation = 'related_to'
				  AND ((source_concept_id = $2 AND target_concept_id = $3)
				    OR (source_concept_id = $3 AND target_concept_id = $2))
				  AND state <> 'rejected'
				ORDER BY created_at LIMIT 1`, userID, source.conceptID, target.conceptID).Scan(&edgeID, &oldState)
		if err == pgx.ErrNoRows {
			err = tx.QueryRow(ctx, `
					INSERT INTO concept_edges (
						user_id, source_concept_id, target_concept_id, relation,
						created_via, state, confidence, last_adapted_at,
						base_confidence, evidence_confidence, observed_at
					) VALUES ($1, $2, $3, 'related_to', 'deterministic_chat', 'candidate', 0.35, now(), 0, 0.35, now())
					RETURNING id, state`, userID, source.conceptID, target.conceptID).Scan(&edgeID, &oldState)
		}
		if err != nil {
			return nil, err
		}

		result, err := tx.Exec(ctx, `
				INSERT INTO adaptive_edge_evidence (
					edge_id, message_id, source_chunk_id, target_chunk_id,
					source_document_id, target_document_id
				) VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (edge_id, message_id) DO NOTHING`, edgeID, messageID,
			source.chunkID, target.chunkID, source.documentID, target.documentID)
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() == 0 {
			continue
		}
		update.LinksObserved++

		var messageCount, documentCount int
		err = tx.QueryRow(ctx, `
				SELECT count(*)::int,
				       (SELECT count(DISTINCT document_id)::int FROM (
				          SELECT source_document_id AS document_id FROM adaptive_edge_evidence WHERE edge_id = $1 AND active
				          UNION ALL
				          SELECT target_document_id FROM adaptive_edge_evidence WHERE edge_id = $1 AND active
				       ) documents)
				FROM adaptive_edge_evidence WHERE edge_id = $1 AND active`, edgeID).Scan(&messageCount, &documentCount)
		if err != nil {
			return nil, err
		}
		newState := adaptiveEdgeState(messageCount, documentCount)
		confidence := adaptiveEdgeConfidence(messageCount, documentCount)
		var savedState string
		err = tx.QueryRow(ctx, `
				UPDATE concept_edges SET
					support_count = $2, document_count = $3,
					evidence_confidence = $4,
					confidence = GREATEST(base_confidence, $4),
					state = CASE WHEN state IN ('confirmed', 'supported') THEN state ELSE $5 END,
					last_adapted_at = now(), observed_at = now(), valid_to = NULL
				WHERE id = $1 RETURNING state`, edgeID, messageCount, documentCount,
			confidence, newState).Scan(&savedState)
		if err != nil {
			return nil, err
		}
		if oldState == "candidate" && savedState == "supported" {
			update.LinksPromoted++
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO chat_graph_updates (
			message_id, concepts_created, concepts_reinforced, links_observed, links_promoted, reducer_version
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (message_id) DO UPDATE SET
			concepts_created = EXCLUDED.concepts_created,
			concepts_reinforced = EXCLUDED.concepts_reinforced,
			links_observed = EXCLUDED.links_observed,
			links_promoted = EXCLUDED.links_promoted,
			reducer_version = EXCLUDED.reducer_version`, messageID, update.ConceptsCreated,
		update.ConceptsReinforced, update.LinksObserved, update.LinksPromoted, update.ReducerVersion)
	return update, err
}

func boundedChatEvidencePairs(evidence []chatConceptEvidence, created map[string]bool, limit int) [][2]chatConceptEvidence {
	pairs := make([][2]chatConceptEvidence, 0, limit)
	appendPair := func(source, target chatConceptEvidence) bool {
		if source.conceptID == target.conceptID || source.chunkID == target.chunkID {
			return false
		}
		pairs = append(pairs, [2]chatConceptEvidence{source, target})
		return len(pairs) >= limit
	}
	if len(created) > 0 {
		for _, source := range evidence {
			if !created[source.conceptID] {
				continue
			}
			for _, target := range evidence {
				if created[target.conceptID] {
					continue
				}
				if appendPair(source, target) {
					return pairs
				}
			}
		}
		return pairs
	}
	for sourceIndex := 0; sourceIndex < len(evidence); sourceIndex++ {
		for targetIndex := sourceIndex + 1; targetIndex < len(evidence); targetIndex++ {
			if appendPair(evidence[sourceIndex], evidence[targetIndex]) {
				return pairs
			}
		}
	}
	return pairs
}

func adaptiveEdgeState(messageCount, documentCount int) string {
	if messageCount >= 3 || (messageCount >= 2 && documentCount >= 2) {
		return "supported"
	}
	return "candidate"
}

func adaptiveEdgeConfidence(messageCount, documentCount int) float64 {
	messageEvidence := math.Min(float64(max(messageCount-1, 0)), 3) * 0.15
	documentEvidence := math.Min(float64(max(documentCount-1, 0)), 2) * 0.10
	return math.Min(0.95, 0.35+messageEvidence+documentEvidence)
}
