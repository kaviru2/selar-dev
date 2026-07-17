-- Source-neutral, research-auditable ingestion for issue #5.
-- Development databases may be recreated; this migration defines the canonical
-- source/snapshot/run model without compatibility views for PDF-only clients.

ALTER TABLE documents
    ADD COLUMN source_id UUID,
    ADD COLUMN source_type TEXT NOT NULL DEFAULT 'pdf'
        CHECK (source_type IN ('pdf', 'web', 'text')),
    ADD COLUMN source_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN canonical_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN mime_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN fetched_at TIMESTAMPTZ;

CREATE TABLE content_sources (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('pdf', 'web', 'text')),
    uri             TEXT NOT NULL DEFAULT '',
    canonical_uri   TEXT NOT NULL DEFAULT '',
    title           TEXT NOT NULL DEFAULT '',
    refresh_policy  TEXT NOT NULL DEFAULT 'manual'
        CHECK (refresh_policy IN ('manual', 'daily', 'weekly', 'never')),
    status          TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'failed', 'archived')),
    config          JSONB NOT NULL DEFAULT '{}',
    last_content_hash TEXT NOT NULL DEFAULT '',
    last_fetched_at TIMESTAMPTZ,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_content_sources_canonical
    ON content_sources(user_id, canonical_uri)
    WHERE canonical_uri <> '' AND status <> 'archived';
CREATE INDEX idx_content_sources_user ON content_sources(user_id, created_at DESC);

ALTER TABLE documents
    ADD CONSTRAINT documents_source_id_fkey
    FOREIGN KEY (source_id) REFERENCES content_sources(id) ON DELETE SET NULL;
CREATE INDEX idx_documents_source ON documents(source_id, added_at DESC);

CREATE TABLE ingestion_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id         UUID NOT NULL REFERENCES content_sources(id) ON DELETE CASCADE,
    document_id       UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status            TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'fetching', 'extracting', 'embedding', 'ready', 'failed', 'unchanged')),
    extractor         TEXT NOT NULL DEFAULT '',
    extractor_version TEXT NOT NULL DEFAULT '',
    embedding_model   TEXT NOT NULL DEFAULT '',
    embedding_dimension INT NOT NULL DEFAULT 3072,
    code_revision     TEXT NOT NULL DEFAULT '',
    metrics           JSONB NOT NULL DEFAULT '{}',
    error             TEXT NOT NULL DEFAULT '',
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ingestion_runs_source ON ingestion_runs(source_id, created_at DESC);
CREATE INDEX idx_ingestion_runs_document ON ingestion_runs(document_id, created_at DESC);

CREATE TABLE content_blocks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id  UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    block_index  INT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('heading', 'paragraph', 'list', 'quote', 'code', 'table', 'figure')),
    text         TEXT NOT NULL DEFAULT '',
    locator      JSONB NOT NULL DEFAULT '{}',
    metadata     JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, block_index)
);

CREATE INDEX idx_content_blocks_document ON content_blocks(document_id, block_index);

ALTER TABLE chunks
    ADD COLUMN locator JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN modality TEXT NOT NULL DEFAULT 'text'
        CHECK (modality IN ('text', 'image', 'mixed')),
    ADD COLUMN embedding_model TEXT NOT NULL DEFAULT 'gemini-embedding-2',
    ADD COLUMN embedding_version TEXT NOT NULL DEFAULT 'v1';

CREATE TABLE assets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id     UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    block_index     INT,
    kind            TEXT NOT NULL
        CHECK (kind IN ('image', 'figure', 'diagram', 'chart', 'table', 'page_render')),
    storage_path    TEXT NOT NULL DEFAULT '',
    source_url      TEXT NOT NULL DEFAULT '',
    mime_type       TEXT NOT NULL DEFAULT '',
    width           INT NOT NULL DEFAULT 0,
    height          INT NOT NULL DEFAULT 0,
    content_hash    TEXT NOT NULL,
    caption         TEXT NOT NULL DEFAULT '',
    alt_text        TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    locator         JSONB NOT NULL DEFAULT '{}',
    embedding       vector(3072),
    embedding_model TEXT NOT NULL DEFAULT 'gemini-embedding-2',
    embedding_version TEXT NOT NULL DEFAULT 'v1',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, content_hash)
);

CREATE INDEX idx_assets_document ON assets(document_id, block_index);

CREATE TABLE chunk_assets (
    chunk_id UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    relation TEXT NOT NULL DEFAULT 'nearby'
        CHECK (relation IN ('contains', 'caption_of', 'explains', 'nearby')),
    PRIMARY KEY (chunk_id, asset_id)
);
