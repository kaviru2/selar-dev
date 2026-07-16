-- Migration 004: Grounded chat episodes, citations, and deterministic retrieval traces.

CREATE TABLE IF NOT EXISTS chat_threads (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL DEFAULT 'New conversation',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_chat_threads_user_updated
    ON chat_threads(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS chat_messages (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id    UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content      TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'complete'
        CHECK (status IN ('pending', 'complete', 'failed')),
    model_version TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_thread_time
    ON chat_messages(thread_id, created_at);

CREATE TABLE IF NOT EXISTS message_citations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    chunk_id    UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    rank        INT NOT NULL,
    score       REAL NOT NULL DEFAULT 0,
    quote       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (message_id, chunk_id)
);

CREATE TABLE IF NOT EXISTS retrieval_traces (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    thread_id         UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    user_message_id   UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    assistant_message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    query             TEXT NOT NULL,
    ranking_policy    TEXT NOT NULL DEFAULT 'hybrid-rrf-v1',
    candidates        JSONB NOT NULL DEFAULT '[]',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE learning_events
    ADD COLUMN IF NOT EXISTS chat_message_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_learning_events_chat_message
    ON learning_events(chat_message_id)
    WHERE chat_message_id IS NOT NULL;
