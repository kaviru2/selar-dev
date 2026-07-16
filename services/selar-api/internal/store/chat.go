package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
		 FROM chat_threads WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
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
		 WHERE t.id = $2 AND t.user_id = $1
		 RETURNING id, thread_id, user_id, role, content, status, model_version, created_at`,
		userID, threadID, role, content, status, modelVersion).Scan(
		&message.ID, &message.ThreadID, &message.UserID, &message.Role, &message.Content,
		&message.Status, &message.ModelVersion, &message.CreatedAt,
	)
	return message, err
}

// ListChatMessages returns a thread with citations while enforcing ownership.
func (s *Store) ListChatMessages(ctx context.Context, userID, threadID string) ([]model.ChatMessage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.thread_id, m.user_id, m.role, m.content, m.status, m.model_version, m.created_at
		 FROM chat_messages m JOIN chat_threads t ON t.id = m.thread_id
		 WHERE t.user_id = $1 AND m.thread_id = $2 ORDER BY m.created_at`, userID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []model.ChatMessage{}
	for rows.Next() {
		var message model.ChatMessage
		message.Citations = []model.ChatCitation{}
		if err := rows.Scan(&message.ID, &message.ThreadID, &message.UserID, &message.Role,
			&message.Content, &message.Status, &message.ModelVersion, &message.CreatedAt); err != nil {
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
	}
	return messages, nil
}

func (s *Store) listMessageCitations(ctx context.Context, messageID string) ([]model.ChatCitation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT mc.id, mc.message_id, mc.chunk_id, c.document_id, d.title, c.page_start,
		        mc.rank, mc.score, mc.quote
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
			&citation.Score, &citation.Quote); err != nil {
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
		 WHERE t.id = $2 AND t.user_id = $1
		 RETURNING id, thread_id, user_id, role, content, status, model_version, created_at`,
		userID, threadID, answer.Answer, answer.ModelVersion).Scan(
		&message.ID, &message.ThreadID, &message.UserID, &message.Role, &message.Content,
		&message.Status, &message.ModelVersion, &message.CreatedAt)
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
		message.Citations = append(message.Citations, saved)
	}

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
