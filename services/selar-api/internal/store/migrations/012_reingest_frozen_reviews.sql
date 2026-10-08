-- Re-ingesting a document deletes its passages (chunks). For a mental-model link
-- that a learner already reviewed, the ON DELETE SET NULL on the evidence chunk
-- ids used to be rejected by both write triggers, so the re-ingest failed (#82).
--
-- The review record must survive a re-ingest unchanged: status, label, review
-- revision/events and the quoted evidence JSON (quote, locator, snapshot hash,
-- text hash) are kept verbatim as a frozen snapshot. Only the pointer to the
-- deleted passage is released. valid_grounded_mental_link() is then false, so
-- the link leaves every active graph/review read until a learner re-reviews new
-- evidence; it is never rebound to new passages.
--
-- The exception is deliberately narrow: an UPDATE may only set an evidence
-- chunk id to NULL when that chunk row no longer exists, and nothing else in the
-- row may change. Nulling live evidence, or any other edit, is still rejected.
CREATE FUNCTION is_dead_evidence_release(old_link mental_model_links, new_link mental_model_links)
RETURNS boolean LANGUAGE sql STABLE AS $$
  SELECT (to_jsonb(new_link) - 'source_evidence_chunk_id' - 'target_evidence_chunk_id')
           = (to_jsonb(old_link) - 'source_evidence_chunk_id' - 'target_evidence_chunk_id')
     AND (new_link.source_evidence_chunk_id IS NOT DISTINCT FROM old_link.source_evidence_chunk_id
          OR (new_link.source_evidence_chunk_id IS NULL
              AND NOT EXISTS (SELECT 1 FROM chunks c WHERE c.id = old_link.source_evidence_chunk_id)))
     AND (new_link.target_evidence_chunk_id IS NOT DISTINCT FROM old_link.target_evidence_chunk_id
          OR (new_link.target_evidence_chunk_id IS NULL
              AND NOT EXISTS (SELECT 1 FROM chunks c WHERE c.id = old_link.target_evidence_chunk_id)))
     AND (new_link.source_evidence_chunk_id IS DISTINCT FROM old_link.source_evidence_chunk_id
          OR new_link.target_evidence_chunk_id IS DISTINCT FROM old_link.target_evidence_chunk_id);
$$;

CREATE OR REPLACE FUNCTION enforce_grounded_mental_link() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND is_dead_evidence_release(OLD, NEW) THEN
    RETURN NEW;
  END IF;
  IF NOT valid_grounded_mental_link(NEW) THEN
    RAISE EXCEPTION 'mental link requires live two-sided owner-matched assertion evidence';
  END IF;
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION lock_reviewed_link_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.review_revision > 0 AND is_dead_evidence_release(OLD, NEW) THEN
    RETURN NEW;
  END IF;
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
