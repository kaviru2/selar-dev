-- Migration 008: provenance-backed concept discovery from grounded chat.

ALTER TABLE chat_graph_updates
    ADD COLUMN IF NOT EXISTS concepts_created INT NOT NULL DEFAULT 0;
