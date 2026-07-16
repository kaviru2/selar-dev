-- Migration 005: Deterministic chat-to-graph reducer and auditable provenance.

ALTER TABLE concept_edges
    ADD COLUMN IF NOT EXISTS support_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS document_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_adapted_at TIMESTAMPTZ;

ALTER TABLE concept_edges DROP CONSTRAINT IF EXISTS concept_edges_created_via_check;
ALTER TABLE concept_edges ADD CONSTRAINT concept_edges_created_via_check
    CHECK (created_via IN ('ai_suggested', 'user_confirmed', 'user_created', 'deterministic_chat'));

CREATE TABLE IF NOT EXISTS chat_concept_evidence (
    message_id  UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    concept_id  UUID NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
    chunk_id    UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    rank        INT NOT NULL,
    score       REAL NOT NULL DEFAULT 0,
    binding_method TEXT NOT NULL DEFAULT 'explicit_chunk_concept',
    binding_confidence REAL NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, concept_id)
);

ALTER TABLE chat_concept_evidence
    ADD COLUMN IF NOT EXISTS binding_method TEXT NOT NULL DEFAULT 'explicit_chunk_concept',
    ADD COLUMN IF NOT EXISTS binding_confidence REAL NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_chat_concept_evidence_concept
    ON chat_concept_evidence(concept_id, created_at DESC);

CREATE TABLE IF NOT EXISTS adaptive_edge_evidence (
    edge_id             UUID NOT NULL REFERENCES concept_edges(id) ON DELETE CASCADE,
    message_id          UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    source_chunk_id     UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    target_chunk_id     UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    source_document_id  UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    target_document_id  UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (edge_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_adaptive_edge_evidence_message
    ON adaptive_edge_evidence(message_id);

CREATE TABLE IF NOT EXISTS chat_graph_updates (
    message_id          UUID PRIMARY KEY REFERENCES chat_messages(id) ON DELETE CASCADE,
    concepts_reinforced INT NOT NULL DEFAULT 0,
    links_observed      INT NOT NULL DEFAULT 0,
    links_promoted      INT NOT NULL DEFAULT 0,
    reducer_version     TEXT NOT NULL DEFAULT 'chat-graph-reducer-v1',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
