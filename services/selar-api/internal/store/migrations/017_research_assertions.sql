-- Issue #9: provenance-aware research assertions.
-- An assertion records what ONE document claims (subject predicate object),
-- with the exact supporting passages, its scope (the document's own work vs.
-- something it reports about another work) and an owner-reviewed lifecycle.
-- Additive only: no existing table is changed. Nothing is created
-- automatically; similarity or co-retrieval can never produce a row.

CREATE TABLE research_assertions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  subject TEXT NOT NULL CHECK (length(btrim(subject)) BETWEEN 1 AND 200),
  subject_key TEXT NOT NULL CHECK (subject_key <> ''),
  subject_qualifier TEXT NOT NULL DEFAULT '',
  predicate TEXT NOT NULL CHECK (predicate IN (
    'introduces', 'uses_benchmark', 'evaluated_on', 'evaluates', 'compared_against',
    'compares', 'reimplemented_as', 'reports_result_for', 'cites')),
  object TEXT NOT NULL CHECK (length(btrim(object)) BETWEEN 1 AND 200),
  object_key TEXT NOT NULL CHECK (object_key <> ''),
  object_qualifier TEXT NOT NULL DEFAULT '',
  asserting_document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  -- own_work: the asserting document describes its own system/experiment.
  -- reported_about_other: it reports something about another work, e.g. a
  -- re-implemented comparison baseline. Only own_work speaks for the subject's
  -- original paper, and only when that paper is the asserting document.
  scope TEXT NOT NULL CHECK (scope IN ('own_work', 'reported_about_other')),
  experiment_context TEXT NOT NULL DEFAULT '' CHECK (length(experiment_context) <= 500),
  confidence REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
  created_via TEXT NOT NULL CHECK (created_via IN ('user_proposed', 'chat_proposal')),
  state TEXT NOT NULL DEFAULT 'proposed'
    CHECK (state IN ('proposed', 'confirmed', 'rejected', 'retracted', 'superseded')),
  superseded_by UUID REFERENCES research_assertions(id) ON DELETE SET NULL,
  source_message_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL,
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  reviewed_at TIMESTAMPTZ
);
CREATE INDEX research_assertions_owner ON research_assertions(user_id, state);
CREATE INDEX research_assertions_subject ON research_assertions(user_id, subject_key, object_key);
CREATE INDEX research_assertions_document ON research_assertions(asserting_document_id);

CREATE TABLE research_assertion_evidence (
  assertion_id UUID NOT NULL REFERENCES research_assertions(id) ON DELETE CASCADE,
  chunk_id UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
  quote TEXT NOT NULL CHECK (length(btrim(quote)) BETWEEN 1 AND 1000),
  text_sha256 TEXT NOT NULL,
  PRIMARY KEY (assertion_id, chunk_id)
);

-- Evidence must be an exact passage of the asserting document, owned by the
-- same user, bound to the chunk text it was quoted from.
CREATE FUNCTION check_research_assertion_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM research_assertions a
    JOIN chunks c ON c.id = NEW.chunk_id
    WHERE a.id = NEW.assertion_id
      AND c.document_id = a.asserting_document_id
      AND c.user_id = a.user_id
      AND strpos(c.content, NEW.quote) > 0
      AND NEW.text_sha256 = encode(digest(c.content, 'sha256'), 'hex')
  ) THEN
    RAISE EXCEPTION 'research assertion evidence must quote the asserting document exactly';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER research_assertion_evidence_check
  BEFORE INSERT OR UPDATE ON research_assertion_evidence
  FOR EACH ROW EXECUTE FUNCTION check_research_assertion_evidence();

-- What a source claims is never rewritten: corrections create a new assertion
-- and supersede the old one. A confirmation needs at least one witness.
CREATE FUNCTION lock_research_assertion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.user_id IS DISTINCT FROM OLD.user_id OR
     NEW.subject IS DISTINCT FROM OLD.subject OR
     NEW.subject_key IS DISTINCT FROM OLD.subject_key OR
     NEW.subject_qualifier IS DISTINCT FROM OLD.subject_qualifier OR
     NEW.predicate IS DISTINCT FROM OLD.predicate OR
     NEW.object IS DISTINCT FROM OLD.object OR
     NEW.object_key IS DISTINCT FROM OLD.object_key OR
     NEW.object_qualifier IS DISTINCT FROM OLD.object_qualifier OR
     NEW.asserting_document_id IS DISTINCT FROM OLD.asserting_document_id OR
     NEW.scope IS DISTINCT FROM OLD.scope OR
     NEW.experiment_context IS DISTINCT FROM OLD.experiment_context OR
     NEW.created_via IS DISTINCT FROM OLD.created_via THEN
    RAISE EXCEPTION 'research assertion content cannot be rewritten; supersede it instead';
  END IF;
  IF NEW.state = 'confirmed' AND OLD.state <> 'confirmed' AND NOT EXISTS (
    SELECT 1 FROM research_assertion_evidence e WHERE e.assertion_id = NEW.id
  ) THEN
    RAISE EXCEPTION 'a research assertion needs source evidence before confirmation';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER research_assertion_lock BEFORE UPDATE ON research_assertions
  FOR EACH ROW EXECUTE FUNCTION lock_research_assertion();

CREATE TABLE research_assertion_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  assertion_id UUID NOT NULL REFERENCES research_assertions(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL CHECK (revision > 0),
  action TEXT NOT NULL CHECK (action IN ('proposed', 'confirmed', 'rejected', 'retracted', 'superseded')),
  before_state TEXT NOT NULL DEFAULT '',
  after_state TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (assertion_id, revision)
);
CREATE TRIGGER immutable_research_assertion_event BEFORE UPDATE ON research_assertion_events
  FOR EACH ROW EXECUTE FUNCTION prevent_review_event_mutation();
