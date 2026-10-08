-- Optional email study notices (issue #114). Additive only.
--
-- * Opt-ins are per notice family and default to off. The server also needs
--   NOTIFY_EMAIL_ENABLED=true and an allowlisted address before any real mail
--   is sent; otherwise notices are recorded as dry-run only.
-- * email_unsubscribe_version is mixed into signed unsubscribe links; using a
--   link turns every notice off and increments it.
-- * notification_log is the audit trail and the idempotency guard: one row per
--   dedupe key (notice kind, learner, quiz, window), claimed before sending, so
--   a window is never notified twice. Recipients are stored as a salted hash,
--   never as an address. Rows are deleted with the account.
ALTER TABLE users
    ADD COLUMN email_quiz_notices BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN email_security_notices BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN email_unsubscribe_version INT NOT NULL DEFAULT 0 CHECK (email_unsubscribe_version >= 0);

CREATE TABLE notification_log (
    id             BIGSERIAL PRIMARY KEY,
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind           TEXT NOT NULL CHECK (kind IN ('quiz_opening', 'quiz_closing_soon',
                                                 'security_password_changed', 'security_email_changed')),
    dedupe_key     TEXT NOT NULL UNIQUE CHECK (length(dedupe_key) BETWEEN 1 AND 300),
    quiz_id        UUID REFERENCES quizzes(id) ON DELETE SET NULL,
    window_start   TIMESTAMPTZ,
    window_end     TIMESTAMPTZ,
    recipient_hash TEXT NOT NULL CHECK (recipient_hash ~ '^[0-9a-f]{64}$'),
    transport      TEXT NOT NULL CHECK (transport IN ('dry-run', 'smtp')),
    subject        TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'dry_run', 'failed')),
    error          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ
);
CREATE INDEX idx_notification_log_user_time ON notification_log (user_id, created_at);
CREATE INDEX idx_notification_log_time ON notification_log (created_at);
