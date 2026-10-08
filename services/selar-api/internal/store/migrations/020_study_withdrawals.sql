-- Anonymous account-withdrawal records retained for study accounting.
-- No user identifier, contact detail, name or participant content is stored.
CREATE TABLE study_withdrawals (
    id                  BIGSERIAL PRIMARY KEY,
    cohort              TEXT,
    group_label         TEXT,
    account_created_at  TIMESTAMPTZ,
    withdrawn_at        TIMESTAMPTZ DEFAULT now(),
    reason              TEXT NOT NULL DEFAULT 'account_deleted'
);
