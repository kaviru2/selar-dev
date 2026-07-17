-- Migration 006: Governed graph lifecycle, feedback, replay, and local metrics.

ALTER TABLE chat_threads
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS retention_policy TEXT NOT NULL DEFAULT 'retain_evidence';

ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS supersedes_message_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL;
ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_status_check;
ALTER TABLE chat_messages ADD CONSTRAINT chat_messages_status_check
    CHECK (status IN ('pending', 'complete', 'failed', 'superseded'));

ALTER TABLE message_citations
    ADD COLUMN IF NOT EXISTS open_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_opened_at TIMESTAMPTZ;

ALTER TABLE concept_edges
    ADD COLUMN IF NOT EXISTS base_confidence REAL NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS evidence_confidence REAL NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS valid_to TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS superseded_by UUID REFERENCES concept_edges(id) ON DELETE SET NULL;

UPDATE concept_edges
SET base_confidence = confidence
WHERE base_confidence = 0 AND created_via <> 'deterministic_chat';

ALTER TABLE concept_edges DROP CONSTRAINT IF EXISTS concept_edges_state_check;
ALTER TABLE concept_edges ADD CONSTRAINT concept_edges_state_check
    CHECK (state IN ('candidate', 'supported', 'confirmed', 'rejected', 'superseded', 'archived'));

ALTER TABLE adaptive_edge_evidence
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMPTZ;

ALTER TABLE chat_concept_evidence
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMPTZ;

ALTER TABLE learning_events
    ADD COLUMN IF NOT EXISTS concept_edge_id UUID REFERENCES concept_edges(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS chat_message_feedback (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id      UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    action          TEXT NOT NULL CHECK (action IN ('helpful', 'unhelpful', 'correction')),
    correction_text TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, message_id, action)
);

CREATE TABLE IF NOT EXISTS citation_interactions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    citation_id   UUID NOT NULL REFERENCES message_citations(id) ON DELETE CASCADE,
    event_type    TEXT NOT NULL DEFAULT 'opened' CHECK (event_type IN ('opened')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, citation_id, event_type)
);

CREATE TABLE IF NOT EXISTS graph_edge_actions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    edge_id        UUID NOT NULL REFERENCES concept_edges(id) ON DELETE CASCADE,
    action         TEXT NOT NULL CHECK (action IN ('confirmed', 'rejected', 'weakened', 'superseded', 'archived')),
    reason         TEXT NOT NULL DEFAULT '',
    source_message_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_graph_edge_actions_edge_time
    ON graph_edge_actions(edge_id, created_at DESC);

CREATE TABLE IF NOT EXISTS chat_learner_projection (
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    concept_id       UUID NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
    exposure_count   INT NOT NULL DEFAULT 0,
    retrieval_count  INT NOT NULL DEFAULT 0,
    success_count    INT NOT NULL DEFAULT 0,
    failure_count    INT NOT NULL DEFAULT 0,
    interest_score   REAL NOT NULL DEFAULT 0,
    last_exposed_at  TIMESTAMPTZ,
    reducer_version  TEXT NOT NULL DEFAULT 'learner-projection-v1',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, concept_id)
);

CREATE TABLE IF NOT EXISTS graph_replay_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reducer_version   TEXT NOT NULL,
    as_of             TIMESTAMPTZ NOT NULL,
    applied           BOOLEAN NOT NULL DEFAULT FALSE,
    differences       INT NOT NULL DEFAULT 0,
    projection_hash   TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS evaluation_metrics (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_message_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL,
    metric_type     TEXT NOT NULL,
    values          JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_evaluation_metrics_user_time
    ON evaluation_metrics(user_id, created_at DESC);
