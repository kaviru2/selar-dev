CREATE TABLE practice_schedule (
 item_id UUID PRIMARY KEY REFERENCES practice_items(id) ON DELETE CASCADE,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 interval_days INTEGER NOT NULL DEFAULT 1 CHECK(interval_days BETWEEN 1 AND 60),
 due_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '1 day',
 last_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 policy TEXT NOT NULL DEFAULT 'doubling-interval-v1'
);
CREATE INDEX practice_due ON practice_schedule(user_id,due_at);
ALTER TABLE practice_attempts ADD COLUMN delayed_unassisted BOOLEAN NOT NULL DEFAULT false;
