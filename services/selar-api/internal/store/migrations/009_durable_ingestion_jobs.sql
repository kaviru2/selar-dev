-- Durable ingestion delivery, retry, and hidden unchanged snapshots.

ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS visible BOOL NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS ingestion_jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id           UUID NOT NULL UNIQUE REFERENCES ingestion_runs(id) ON DELETE CASCADE,
    source_id        UUID NOT NULL REFERENCES content_sources(id) ON DELETE CASCADE,
    document_id      UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type      TEXT NOT NULL CHECK (source_type IN ('pdf', 'web', 'text')),
    file_path        TEXT NOT NULL DEFAULT '',
    source_url       TEXT NOT NULL DEFAULT '',
    raw_text         TEXT NOT NULL DEFAULT '',
    title            TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'leased', 'completed', 'failed', 'cancelled')),
    attempts         INT NOT NULL DEFAULT 0,
    max_attempts     INT NOT NULL DEFAULT 3,
    available_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_expires_at TIMESTAMPTZ,
    worker_id        TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_claim
    ON ingestion_jobs(status, available_at, created_at);
CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_document
    ON ingestion_jobs(document_id, created_at DESC);

