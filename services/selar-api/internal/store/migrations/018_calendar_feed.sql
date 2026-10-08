-- Optional quiz-window calendar feed (issue #113). Additive only.
--
-- * calendar_feed_enabled is the learner's own opt-in from Settings (default
--   off). The server additionally requires CALENDAR_FEED_ENABLED=true and an
--   allowlisted address before a feed shows any event.
-- * calendar_feed_version is mixed into the signed feed URL. Resetting the
--   link or turning the feed off increments it, which revokes every URL issued
--   before. No token is stored.
ALTER TABLE users
    ADD COLUMN calendar_feed_enabled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN calendar_feed_version INT NOT NULL DEFAULT 0 CHECK (calendar_feed_version >= 0);
