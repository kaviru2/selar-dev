-- Migration 003: Runtime-expanding mental models and deterministic learner state.

CREATE TABLE IF NOT EXISTS document_mental_models (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id     UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    version         INT NOT NULL DEFAULT 1,
    main_claim      TEXT NOT NULL,
    key_concepts    TEXT[] NOT NULL DEFAULT '{}',
    assumptions     TEXT[] NOT NULL DEFAULT '{}',
    open_questions  TEXT[] NOT NULL DEFAULT '{}',
    domain          TEXT NOT NULL DEFAULT '',
    embedding       vector(3072),
    model_version   TEXT NOT NULL DEFAULT '',
    prompt_version  TEXT NOT NULL DEFAULT 'mental-model-v1',
    status          TEXT NOT NULL DEFAULT 'ready'
        CHECK (status IN ('draft', 'ready', 'failed', 'superseded')),
    generated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, version)
);

CREATE INDEX IF NOT EXISTS idx_mental_models_user
    ON document_mental_models(user_id, generated_at DESC);
CREATE INDEX IF NOT EXISTS idx_mental_models_document
    ON document_mental_models(document_id, version DESC);

CREATE TABLE IF NOT EXISTS mental_model_links (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_model_id          UUID NOT NULL REFERENCES document_mental_models(id) ON DELETE CASCADE,
    target_model_id          UUID NOT NULL REFERENCES document_mental_models(id) ON DELETE CASCADE,
    link_type                TEXT NOT NULL
        CHECK (link_type IN ('concept_overlap', 'claim_extension', 'assumption_conflict', 'question_resolution')),
    similarity               REAL NOT NULL DEFAULT 0,
    confidence               REAL NOT NULL DEFAULT 0,
    bridge_explanation       TEXT NOT NULL DEFAULT '',
    source_evidence_chunk_id UUID REFERENCES chunks(id) ON DELETE SET NULL,
    target_evidence_chunk_id UUID REFERENCES chunks(id) ON DELETE SET NULL,
    status                   TEXT NOT NULL DEFAULT 'candidate'
        CHECK (status IN ('candidate', 'confirmed', 'rejected', 'relabeled', 'archived')),
    created_via              TEXT NOT NULL DEFAULT 'ai_suggested'
        CHECK (created_via IN ('ai_suggested', 'user_confirmed', 'user_created')),
    model_version            TEXT NOT NULL DEFAULT '',
    prompt_version           TEXT NOT NULL DEFAULT 'mental-link-v1',
    user_label               TEXT,
    suggested_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at             TIMESTAMPTZ,
    UNIQUE (source_model_id, target_model_id, link_type)
);

CREATE INDEX IF NOT EXISTS idx_mental_links_user_status
    ON mental_model_links(user_id, status, suggested_at DESC);
CREATE INDEX IF NOT EXISTS idx_mental_links_source ON mental_model_links(source_model_id);
CREATE INDEX IF NOT EXISTS idx_mental_links_target ON mental_model_links(target_model_id);

CREATE TABLE IF NOT EXISTS learning_events (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type       TEXT NOT NULL,
    occurred_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    document_id      UUID REFERENCES documents(id) ON DELETE SET NULL,
    chunk_id         UUID REFERENCES chunks(id) ON DELETE SET NULL,
    concept_id       UUID REFERENCES concepts(id) ON DELETE SET NULL,
    suggestion_id    UUID REFERENCES link_suggestions(id) ON DELETE SET NULL,
    mental_link_id   UUID REFERENCES mental_model_links(id) ON DELETE SET NULL,
    payload          JSONB NOT NULL DEFAULT '{}',
    source           TEXT NOT NULL DEFAULT 'api',
    schema_version   INT NOT NULL DEFAULT 1,
    idempotency_key  TEXT NOT NULL,
    UNIQUE (user_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_learning_events_user_time
    ON learning_events(user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_learning_events_concept_time
    ON learning_events(user_id, concept_id, occurred_at DESC)
    WHERE concept_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS learner_concept_state (
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    concept_id           UUID NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
    mastery_estimate     REAL NOT NULL DEFAULT 0.25,
    half_life_seconds    DOUBLE PRECISION NOT NULL DEFAULT 86400,
    last_exposed_at      TIMESTAMPTZ,
    last_retrieved_at    TIMESTAMPTZ,
    success_count        INT NOT NULL DEFAULT 0,
    failure_count        INT NOT NULL DEFAULT 0,
    evidence_count       INT NOT NULL DEFAULT 0,
    uncertainty          REAL NOT NULL DEFAULT 1,
    state_version        INT NOT NULL DEFAULT 1,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, concept_id)
);

ALTER TABLE concepts ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'candidate'
    CHECK (state IN ('candidate', 'supported', 'confirmed', 'rejected', 'archived'));
ALTER TABLE concepts ADD COLUMN IF NOT EXISTS model_version TEXT NOT NULL DEFAULT '';
ALTER TABLE concepts ADD COLUMN IF NOT EXISTS prompt_version TEXT NOT NULL DEFAULT 'concept-v1';

ALTER TABLE concept_edges ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'candidate'
    CHECK (state IN ('candidate', 'supported', 'confirmed', 'rejected', 'archived'));
ALTER TABLE concept_edges ADD COLUMN IF NOT EXISTS confidence REAL NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw
    ON chunks USING hnsw ((embedding::halfvec(3072)) halfvec_cosine_ops)
    WITH (m = 16, ef_construction = 64);
CREATE INDEX IF NOT EXISTS idx_concepts_embedding_hnsw
    ON concepts USING hnsw ((embedding::halfvec(3072)) halfvec_cosine_ops)
    WITH (m = 16, ef_construction = 64);
CREATE INDEX IF NOT EXISTS idx_mental_models_embedding_hnsw
    ON document_mental_models USING hnsw ((embedding::halfvec(3072)) halfvec_cosine_ops)
    WITH (m = 16, ef_construction = 64);
