-- SELAR Schema v2 — Postgres 16 + pgvector
-- Full schema from implementation spec. Embedding vectors use 3072 dimensions.
-- All coordinates stored in PDF points (1/72 inch), never screen pixels.
-- IMPORTANT: Run this migration with a fresh database or drop existing tables first.

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================
-- Users
-- ============================================================
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    cohort          TEXT NOT NULL DEFAULT 'control'
        CHECK (cohort IN ('control', 'treatment_auto', 'treatment_hitl')),
    drive_connected BOOL NOT NULL DEFAULT false,
    preferences     JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- Documents — metadata only; PDFs stay in user's Drive or local storage
-- ============================================================
CREATE TABLE documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    authors         TEXT NOT NULL DEFAULT '',
    year            INT NOT NULL DEFAULT 0,
    page_count      INT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'uploaded'
        CHECK (status IN ('uploaded', 'processing', 'ready', 'failed')),
    progress        REAL NOT NULL DEFAULT 0,
    file_path       TEXT NOT NULL DEFAULT '',
    gdrive_file_id  TEXT NOT NULL DEFAULT '',
    added_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX idx_documents_user ON documents(user_id);

-- ============================================================
-- Chunks — text segments with vector embeddings and bbox coords
-- ============================================================
CREATE TABLE chunks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id  UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chunk_index  INT NOT NULL,
    page_start   INT NOT NULL DEFAULT 1,
    page_end     INT NOT NULL DEFAULT 1,
    content      TEXT NOT NULL,
    token_count  INT NOT NULL DEFAULT 0,
    embedding    vector(3072),
    bboxes       JSONB NOT NULL DEFAULT '[]',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_chunks_doc ON chunks(document_id);
CREATE INDEX idx_chunks_user ON chunks(user_id);
CREATE INDEX idx_chunks_doc_page ON chunks(document_id, page_start);
-- HNSW index for fast approximate kNN (create after initial data load for best build quality)
-- CREATE INDEX idx_chunks_embedding ON chunks USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);

-- ============================================================
-- Concepts — nodes in the lightweight knowledge graph
-- ============================================================
CREATE TABLE concepts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    embedding   vector(3072),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, name)
);

-- ============================================================
-- Chunk–Concept junction
-- ============================================================
CREATE TABLE chunk_concepts (
    chunk_id    UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    concept_id  UUID NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
    confidence  REAL NOT NULL DEFAULT 1.0,
    PRIMARY KEY (chunk_id, concept_id)
);

-- ============================================================
-- Concept edges — the knowledge graph edges (replaces Neo4j)
-- ============================================================
CREATE TABLE concept_edges (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_concept_id  UUID NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
    target_concept_id  UUID NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
    relation           TEXT NOT NULL
        CHECK (relation IN ('prerequisite_of', 'related_to', 'sub_concept_of', 'contradicts', 'extends')),
    created_via        TEXT NOT NULL DEFAULT 'ai_suggested'
        CHECK (created_via IN ('ai_suggested', 'user_confirmed', 'user_created')),
    confirmed_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_concept_id, target_concept_id, relation)
);

-- ============================================================
-- Link suggestions — THE core table for the study intervention
-- ============================================================
CREATE TABLE link_suggestions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_chunk_id    UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    target_chunk_id    UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    similarity         REAL NOT NULL,
    relation           TEXT NOT NULL DEFAULT 'related_to'
        CHECK (relation IN ('related_to', 'prerequisite_of', 'sub_concept_of', 'contradicts', 'extends')),
    status             TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'confirmed', 'rejected', 'relabeled', 'expired')),
    user_label         TEXT,
    time_to_respond_ms INT,
    suggested_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at       TIMESTAMPTZ
);

CREATE INDEX idx_suggestions_user_status ON link_suggestions(user_id, status);
CREATE INDEX idx_suggestions_source ON link_suggestions(source_chunk_id);
CREATE INDEX idx_suggestions_target ON link_suggestions(target_chunk_id);

-- ============================================================
-- Annotations — user highlights and notes on PDF pages
-- All coordinates in PDF points (1/72 inch), origin at bottom-left.
-- The PDF itself is NEVER modified — these render as overlay <div>s.
-- ============================================================
CREATE TABLE annotations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_id  UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    chunk_id     UUID REFERENCES chunks(id) ON DELETE SET NULL,
    page         INT NOT NULL,
    bbox         JSONB NOT NULL,
    color        TEXT NOT NULL DEFAULT 'wheat'
        CHECK (color IN ('wheat', 'yellow', 'coral', 'sage')),
    type         TEXT NOT NULL
        CHECK (type IN ('highlight', 'underline', 'note', 'suggestion')),
    comment      TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_annotations_doc_page ON annotations(document_id, page);
CREATE INDEX idx_annotations_user ON annotations(user_id);

-- ============================================================
-- Reading sessions — behavioral data for analysis
-- ============================================================
CREATE TABLE reading_sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_id      UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at         TIMESTAMPTZ,
    pages_viewed     INT[] NOT NULL DEFAULT '{}',
    max_scroll_depth INT NOT NULL DEFAULT 0
);

-- ============================================================
-- Quizzes — retention test module
-- ============================================================
CREATE TABLE quizzes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    phase       TEXT NOT NULL CHECK (phase IN ('pre', 'post', 'delayed')),
    corpus_tag  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE quiz_questions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_id         UUID NOT NULL REFERENCES quizzes(id) ON DELETE CASCADE,
    question_index  INT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('mcq', 'short_answer', 'cloze', 'connection')),
    prompt          TEXT NOT NULL,
    options         JSONB,
    correct_answer  TEXT NOT NULL,
    concept_tags    TEXT[] NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE quiz_attempts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    quiz_id      UUID NOT NULL REFERENCES quizzes(id) ON DELETE CASCADE,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    submitted_at TIMESTAMPTZ,
    score        REAL
);

CREATE TABLE quiz_responses (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id        UUID NOT NULL REFERENCES quiz_attempts(id) ON DELETE CASCADE,
    question_id       UUID NOT NULL REFERENCES quiz_questions(id) ON DELETE CASCADE,
    user_answer       TEXT,
    is_correct        BOOL,
    time_to_answer_ms INT
);
