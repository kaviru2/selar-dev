-- A reviewed candidate remains the one grounded mental-model link, not a parallel edge.
ALTER TABLE mental_model_links ADD COLUMN review_revision BIGINT NOT NULL DEFAULT 0;

CREATE TABLE mental_link_review_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  link_id UUID NOT NULL REFERENCES mental_model_links(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL CHECK (revision > 0),
  action TEXT NOT NULL CHECK (action IN ('confirmed','rejected','relabeled','retracted','rolled_back')),
  before_status TEXT NOT NULL,
  after_status TEXT NOT NULL,
  before_label TEXT,
  after_label TEXT,
  reason TEXT NOT NULL DEFAULT '',
  target_revision BIGINT,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(link_id,revision)
);
CREATE INDEX mental_link_review_events_owner ON mental_link_review_events(user_id,link_id,revision);

CREATE FUNCTION prevent_review_event_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'review audit events are append-only';
END;
$$;
-- Deletion must remain possible through owner/document erasure cascades.
CREATE TRIGGER immutable_mental_review BEFORE UPDATE ON mental_link_review_events
  FOR EACH ROW EXECUTE FUNCTION prevent_review_event_mutation();

-- No writer may replace the source assertion, direction, or snapshot after a human review.
-- A changed document snapshot still invalidates it via valid_grounded_mental_link().
CREATE FUNCTION lock_reviewed_link_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.review_revision > 0 AND (
    NEW.user_id IS DISTINCT FROM OLD.user_id OR
    NEW.source_model_id IS DISTINCT FROM OLD.source_model_id OR
    NEW.target_model_id IS DISTINCT FROM OLD.target_model_id OR
    NEW.link_type IS DISTINCT FROM OLD.link_type OR
    NEW.source_evidence_chunk_id IS DISTINCT FROM OLD.source_evidence_chunk_id OR
    NEW.target_evidence_chunk_id IS DISTINCT FROM OLD.target_evidence_chunk_id OR
    NEW.source_evidence IS DISTINCT FROM OLD.source_evidence OR
    NEW.target_evidence IS DISTINCT FROM OLD.target_evidence
  ) THEN
    RAISE EXCEPTION 'reviewed link evidence cannot be replaced';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER reviewed_link_evidence_lock BEFORE UPDATE ON mental_model_links
 FOR EACH ROW EXECUTE FUNCTION lock_reviewed_link_evidence();
