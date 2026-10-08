-- Settings page: profile display name, server-side session invalidation and
-- per-cohort locks for settings that could change a study condition.

ALTER TABLE users
    ADD COLUMN display_name TEXT NOT NULL DEFAULT ''
        CHECK (char_length(display_name) <= 80),
    -- Every JWT carries the session version it was issued under; the API
    -- rejects tokens whose version is older. A password change increments it,
    -- ending every other signed-in session. Tokens issued before this column
    -- existed carry no version and count as 0, so they stay valid.
    ADD COLUMN session_version INT NOT NULL DEFAULT 0 CHECK (session_version >= 0);

-- An administrator can pin a study-sensitive setting for a whole cohort. The
-- API only honours keys the settings package marks as lockable.
CREATE TABLE cohort_setting_locks (
    cohort      TEXT NOT NULL
        CHECK (cohort IN ('control', 'treatment_auto', 'treatment_hitl')),
    setting_key TEXT NOT NULL CHECK (setting_key <> ''),
    value       JSONB NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cohort, setting_key)
);
