package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/selar-dev/selar-api/internal/model"
)

var (
	ErrResearchAssertionNotFound = errors.New("research assertion not found")
	ErrResearchAssertionInvalid  = errors.New("invalid research assertion")
	ErrResearchAssertionStale    = errors.New("stale research assertion revision")
	ErrResearchAssertionEvidence = errors.New("evidence must quote the asserting document exactly")
)

// ValidateResearchAssertionInput checks the closed ontology and required provenance.
func ValidateResearchAssertionInput(in *model.ResearchAssertionInput) error {
	in.Subject = strings.TrimSpace(in.Subject)
	in.Object = strings.TrimSpace(in.Object)
	in.Predicate = strings.TrimSpace(in.Predicate)
	in.Scope = strings.TrimSpace(in.Scope)
	in.ExperimentContext = strings.TrimSpace(in.ExperimentContext)
	if _, ok := model.ResearchPredicates[in.Predicate]; !ok {
		return fmt.Errorf("%w: unknown predicate", ErrResearchAssertionInvalid)
	}
	if in.Scope != "own_work" && in.Scope != "reported_about_other" {
		return fmt.Errorf("%w: scope must be own_work or reported_about_other", ErrResearchAssertionInvalid)
	}
	if in.Subject == "" || in.Object == "" || len(in.Subject) > 200 || len(in.Object) > 200 || len(in.ExperimentContext) > 500 {
		return fmt.Errorf("%w: subject/object length", ErrResearchAssertionInvalid)
	}
	if sk, _ := model.NormalizeEntity(in.Subject); sk == "" {
		return fmt.Errorf("%w: subject has no name", ErrResearchAssertionInvalid)
	}
	if ok, _ := model.NormalizeEntity(in.Object); ok == "" {
		return fmt.Errorf("%w: object has no name", ErrResearchAssertionInvalid)
	}
	if in.AssertingDocumentID == "" {
		return fmt.Errorf("%w: asserting document is required", ErrResearchAssertionInvalid)
	}
	if len(in.Evidence) == 0 || len(in.Evidence) > 5 {
		return fmt.Errorf("%w: 1 to 5 evidence passages are required", ErrResearchAssertionInvalid)
	}
	for i := range in.Evidence {
		in.Evidence[i].Quote = strings.TrimSpace(in.Evidence[i].Quote)
		if in.Evidence[i].ChunkID == "" || in.Evidence[i].Quote == "" || len(in.Evidence[i].Quote) > 1000 {
			return fmt.Errorf("%w: evidence needs a chunk and quote", ErrResearchAssertionInvalid)
		}
	}
	if in.Confidence != nil && (*in.Confidence < 0 || *in.Confidence > 1) {
		return fmt.Errorf("%w: confidence must be in [0,1]", ErrResearchAssertionInvalid)
	}
	return nil
}

func insertResearchAssertion(ctx context.Context, tx pgx.Tx, userID string, in model.ResearchAssertionInput, createdVia string) (string, error) {
	subjectKey, subjectQualifier := model.NormalizeEntity(in.Subject)
	objectKey, objectQualifier := model.NormalizeEntity(in.Object)
	confidence := float32(0.5)
	if in.Confidence != nil {
		confidence = *in.Confidence
	}
	var sourceMessage any
	if in.SourceMessageID != "" {
		sourceMessage = in.SourceMessageID
	}
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO research_assertions (
			user_id, subject, subject_key, subject_qualifier, predicate, object, object_key, object_qualifier,
			asserting_document_id, scope, experiment_context, confidence, created_via, source_message_id)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, d.id, $10, $11, $12, $13,
		       (SELECT m.id FROM chat_messages m WHERE m.id = $14::uuid AND m.user_id = $1)
		FROM documents d WHERE d.id = $9 AND d.user_id = $1 AND d.status = 'ready'
		RETURNING id`, userID, in.Subject, subjectKey, subjectQualifier, in.Predicate, in.Object,
		objectKey, objectQualifier, in.AssertingDocumentID, in.Scope, in.ExperimentContext,
		confidence, createdVia, sourceMessage).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", ErrResearchAssertionNotFound
	}
	if err != nil {
		return "", err
	}
	for _, ev := range in.Evidence {
		var content string
		err := tx.QueryRow(ctx, `SELECT content FROM chunks WHERE id = $1 AND user_id = $2 AND document_id = $3`,
			ev.ChunkID, userID, in.AssertingDocumentID).Scan(&content)
		if err == pgx.ErrNoRows || (err == nil && !strings.Contains(content, ev.Quote)) {
			return "", ErrResearchAssertionEvidence
		}
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256([]byte(content))
		if _, err := tx.Exec(ctx, `
			INSERT INTO research_assertion_evidence (assertion_id, chunk_id, quote, text_sha256)
			VALUES ($1, $2, $3, $4)`, id, ev.ChunkID, ev.Quote, hex.EncodeToString(sum[:])); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO research_assertion_events (user_id, assertion_id, revision, action, after_state)
		VALUES ($1, $2, 1, 'proposed', 'proposed')`, userID, id); err != nil {
		return "", err
	}
	return id, nil
}

// ProposeResearchAssertion stores a learner-proposed assertion in state
// "proposed". It never confirms anything: confirmation is a separate review.
func (s *Store) ProposeResearchAssertion(ctx context.Context, userID string, in model.ResearchAssertionInput) (*model.ResearchAssertion, error) {
	if err := ValidateResearchAssertionInput(&in); err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	createdVia := "user_proposed"
	if in.SourceMessageID != "" {
		createdVia = "chat_proposal"
	}
	id, err := insertResearchAssertion(ctx, tx, userID, in, createdVia)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetResearchAssertion(ctx, userID, id)
}

const researchAssertionColumns = `a.id, a.subject, a.subject_key, a.subject_qualifier, a.predicate, a.object,
	a.object_key, a.object_qualifier, a.asserting_document_id, d.title, a.scope, a.experiment_context,
	a.confidence, a.created_via, a.state, COALESCE(a.superseded_by::text, ''),
	COALESCE(a.source_message_id::text, ''), a.revision, a.created_at, a.reviewed_at`

func scanResearchAssertion(row pgx.Row) (*model.ResearchAssertion, error) {
	a := &model.ResearchAssertion{Evidence: []model.ResearchAssertionEvidence{}}
	err := row.Scan(&a.ID, &a.Subject, &a.SubjectKey, &a.SubjectQualifier, &a.Predicate, &a.Object,
		&a.ObjectKey, &a.ObjectQualifier, &a.AssertingDocumentID, &a.AssertingDocument, &a.Scope,
		&a.ExperimentContext, &a.Confidence, &a.CreatedVia, &a.State, &a.SupersededBy,
		&a.SourceMessageID, &a.Revision, &a.CreatedAt, &a.ReviewedAt)
	return a, err
}

func (s *Store) loadResearchEvidence(ctx context.Context, userID string, items []*model.ResearchAssertion) error {
	for _, a := range items {
		// Evidence whose chunk text changed since quoting is not shown as support.
		rows, err := s.pool.Query(ctx, `
			SELECT e.chunk_id, e.quote, c.page_start FROM research_assertion_evidence e
			JOIN chunks c ON c.id = e.chunk_id AND c.user_id = $2
			WHERE e.assertion_id = $1 AND e.text_sha256 = encode(digest(c.content, 'sha256'), 'hex')
			ORDER BY e.chunk_id`, a.ID, userID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var ev model.ResearchAssertionEvidence
			if err := rows.Scan(&ev.ChunkID, &ev.Quote, &ev.Page); err != nil {
				rows.Close()
				return err
			}
			a.Evidence = append(a.Evidence, ev)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

// GetResearchAssertion returns one owner-scoped assertion with its live evidence.
func (s *Store) GetResearchAssertion(ctx context.Context, userID, id string) (*model.ResearchAssertion, error) {
	a, err := scanResearchAssertion(s.pool.QueryRow(ctx, `SELECT `+researchAssertionColumns+`
		FROM research_assertions a JOIN documents d ON d.id = a.asserting_document_id
		WHERE a.id = $1 AND a.user_id = $2`, id, userID))
	if err == pgx.ErrNoRows {
		return nil, ErrResearchAssertionNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, s.loadResearchEvidence(ctx, userID, []*model.ResearchAssertion{a})
}

// ListResearchAssertions lists owner assertions, optionally filtered by state
// and by an entity name (matched by normalized key on either side).
func (s *Store) ListResearchAssertions(ctx context.Context, userID, state, entity string) ([]model.ResearchAssertion, error) {
	key := ""
	if entity != "" {
		key, _ = model.NormalizeEntity(entity)
	}
	rows, err := s.pool.Query(ctx, `SELECT `+researchAssertionColumns+`
		FROM research_assertions a JOIN documents d ON d.id = a.asserting_document_id
		WHERE a.user_id = $1 AND ($2 = '' OR a.state = $2)
		  AND ($3 = '' OR a.subject_key = $3 OR a.object_key = $3)
		ORDER BY a.created_at, a.id`, userID, state, key)
	if err != nil {
		return nil, err
	}
	var items []*model.ResearchAssertion
	for rows.Next() {
		a, err := scanResearchAssertion(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.loadResearchEvidence(ctx, userID, items); err != nil {
		return nil, err
	}
	out := make([]model.ResearchAssertion, 0, len(items))
	for _, a := range items {
		out = append(out, *a)
	}
	return out, nil
}

var researchTransitions = map[string]map[string]string{
	"confirm":   {"proposed": "confirmed"},
	"reject":    {"proposed": "rejected"},
	"retract":   {"confirmed": "retracted", "proposed": "retracted"},
	"supersede": {"confirmed": "superseded", "proposed": "superseded"},
}

// RespondToResearchAssertion applies one owner review action bound to the
// revision the owner saw. "supersede" requires a correction, which is stored
// as a new proposed assertion; the original source claim row is never edited.
func (s *Store) RespondToResearchAssertion(ctx context.Context, userID, id string, action model.ResearchAssertionAction) (*model.ResearchAssertion, error) {
	transitions, ok := researchTransitions[action.Action]
	if !ok || len(action.Reason) > 500 {
		return nil, fmt.Errorf("%w: unknown action", ErrResearchAssertionInvalid)
	}
	if action.Action == "supersede" {
		if action.Correction == nil {
			return nil, fmt.Errorf("%w: supersede requires a correction", ErrResearchAssertionInvalid)
		}
		if err := ValidateResearchAssertionInput(action.Correction); err != nil {
			return nil, err
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var state string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT state, revision FROM research_assertions WHERE id = $1 AND user_id = $2 FOR UPDATE`,
		id, userID).Scan(&state, &revision)
	if err == pgx.ErrNoRows {
		return nil, ErrResearchAssertionNotFound
	}
	if err != nil {
		return nil, err
	}
	if action.Revision != revision {
		return nil, ErrResearchAssertionStale
	}
	next, ok := transitions[state]
	if !ok {
		return nil, fmt.Errorf("%w: cannot %s a %s assertion", ErrResearchAssertionInvalid, action.Action, state)
	}
	if next == "confirmed" {
		var live int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM research_assertion_evidence e
			JOIN research_assertions a ON a.id = e.assertion_id
			JOIN chunks c ON c.id = e.chunk_id AND c.user_id = a.user_id AND c.document_id = a.asserting_document_id
			JOIN documents d ON d.id = c.document_id AND d.status = 'ready'
			WHERE e.assertion_id = $1 AND e.text_sha256 = encode(digest(c.content, 'sha256'), 'hex')`, id).Scan(&live); err != nil {
			return nil, err
		}
		if live == 0 {
			return nil, ErrResearchAssertionEvidence
		}
	}
	var supersededBy any
	if action.Action == "supersede" {
		correctionID, err := insertResearchAssertion(ctx, tx, userID, *action.Correction, "user_proposed")
		if err != nil {
			return nil, err
		}
		supersededBy = correctionID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE research_assertions SET state = $3, revision = revision + 1, reviewed_at = $4,
		       superseded_by = COALESCE($5::uuid, superseded_by)
		WHERE id = $1 AND user_id = $2`, id, userID, next, time.Now().UTC(), supersededBy); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO research_assertion_events (user_id, assertion_id, revision, action, before_state, after_state, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, userID, id, revision+1, next, state, next, strings.TrimSpace(action.Reason)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetResearchAssertion(ctx, userID, id)
}

// ListConfirmedResearchAssertionEdges projects only owner-confirmed assertions
// with live evidence into the graph. The asserting document is part of every
// edge, so "paper B evaluated a modified A on C" never becomes "A used C".
func (s *Store) ListConfirmedResearchAssertionEdges(ctx context.Context, userID string) ([]model.ResearchAssertion, error) {
	items, err := s.ListResearchAssertions(ctx, userID, "confirmed", "")
	if err != nil {
		return nil, err
	}
	out := items[:0]
	for _, a := range items {
		if len(a.Evidence) > 0 {
			out = append(out, a)
		}
	}
	return out, nil
}
