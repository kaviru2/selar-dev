-- Admin roles, optional research consent and first-party usage analytics (#84).
--
-- * users.role gates the /api/admin routes ('user' by default). It is read from
--   the database on every admin request, never trusted from the JWT.
-- * users.group_label is a neutral, admin-editable label (for example to tell
--   two study conditions apart). It is NOT the legacy users.cohort column and
--   it does not change application behaviour.
-- * consented_at / consent_version record the optional in-app opt-in to usage
--   analytics. consent_decided_at records that the user answered the prompt
--   (yes or no) so it is not shown again. This is product consent only and is
--   not a substitute for ethics-approved research consent.
-- * analytics_events stores per-user events ONLY for users who opted in.
--   Deleting a user deletes their events (ON DELETE CASCADE).
-- * analytics_daily_counts keeps anonymous per-day totals (no user id) so the
--   dashboard can show overall usage without tracking people who did not opt in.
ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    ADD COLUMN group_label TEXT NOT NULL DEFAULT ''
        CHECK (length(group_label) <= 64 AND group_label !~ '[[:cntrl:]]'),
    ADD COLUMN consented_at TIMESTAMPTZ,
    ADD COLUMN consent_version TEXT,
    ADD COLUMN consent_decided_at TIMESTAMPTZ,
    ADD CONSTRAINT users_consent_version_with_consent
        CHECK ((consented_at IS NULL) = (consent_version IS NULL));

CREATE INDEX idx_users_group_label ON users (group_label) WHERE group_label <> '';

CREATE TABLE analytics_events (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event       TEXT NOT NULL CHECK (event ~ '^[a-z][a-z0-9_]{1,63}$'),
    props       JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(props) = 'object'),
    source      TEXT NOT NULL CHECK (source IN ('server', 'client')),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_analytics_events_time ON analytics_events (occurred_at);
CREATE INDEX idx_analytics_events_user_time ON analytics_events (user_id, occurred_at);
CREATE INDEX idx_analytics_events_event_time ON analytics_events (event, occurred_at);

CREATE TABLE analytics_daily_counts (
    day   DATE NOT NULL,
    event TEXT NOT NULL CHECK (event ~ '^[a-z][a-z0-9_]{1,63}$'),
    count BIGINT NOT NULL DEFAULT 0 CHECK (count >= 0),
    PRIMARY KEY (day, event)
);
