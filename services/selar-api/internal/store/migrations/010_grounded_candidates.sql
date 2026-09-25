-- Candidate evidence is an assertion by each source, not an embedding-distance label.
-- Legacy candidate rows remain but are invisible to the grounded read/review path.
ALTER TABLE mental_model_links
  ADD COLUMN source_evidence JSONB,
  ADD COLUMN target_evidence JSONB;

CREATE FUNCTION valid_grounded_mental_link(link mental_model_links) RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (
    SELECT 1 FROM document_mental_models sm
    JOIN document_mental_models tm ON tm.id = link.target_model_id
    JOIN documents sd ON sd.id = sm.document_id
    JOIN documents td ON td.id = tm.document_id
    JOIN chunks sc ON sc.id = link.source_evidence_chunk_id
    JOIN chunks tc ON tc.id = link.target_evidence_chunk_id
    WHERE sm.id = link.source_model_id
      AND link.link_type = 'concept_overlap'
      AND sm.status = 'ready' AND tm.status = 'ready'
      AND sd.status IN ('processing', 'ready') AND td.status = 'ready'
      AND sm.document_id <> tm.document_id
      AND sm.user_id = link.user_id AND tm.user_id = link.user_id
      AND sd.user_id = link.user_id AND td.user_id = link.user_id
      AND sc.user_id = link.user_id AND tc.user_id = link.user_id
      AND sc.document_id = sd.id AND tc.document_id = td.id
      AND link.source_evidence->>'chunk_id' = sc.id::text
      AND link.target_evidence->>'chunk_id' = tc.id::text
      AND link.source_evidence->>'asserting_source_id' = sd.id::text
      AND link.target_evidence->>'asserting_source_id' = td.id::text
      AND sd.content_hash <> '' AND td.content_hash <> ''
      AND link.source_evidence->>'source_snapshot_hash' = sd.content_hash
      AND link.target_evidence->>'source_snapshot_hash' = td.content_hash
      AND length(coalesce(link.source_evidence->>'asserted_concept', '')) >= 12
      AND position(' ' IN link.source_evidence->>'asserted_concept') > 0
      AND lower(link.source_evidence->>'asserted_concept') = lower(link.target_evidence->>'asserted_concept')
      AND EXISTS (SELECT 1 FROM unnest(sm.key_concepts) concept
                  WHERE lower(concept) = lower(link.source_evidence->>'asserted_concept'))
      AND position(lower(link.source_evidence->>'asserted_concept') IN lower(link.source_evidence->>'quote')) > 0
      AND position(lower(link.target_evidence->>'asserted_concept') IN lower(link.target_evidence->>'quote')) > 0
      AND sc.locator <> '{}'::jsonb AND tc.locator <> '{}'::jsonb
      AND link.source_evidence->'locator' = sc.locator
      AND link.target_evidence->'locator' = tc.locator
      AND length(coalesce(link.source_evidence->>'quote', '')) >= 12
      AND length(coalesce(link.target_evidence->>'quote', '')) >= 12
      AND position(link.source_evidence->>'quote' IN sc.content) > 0
      AND position(link.target_evidence->>'quote' IN tc.content) > 0
      AND link.source_evidence->>'text_sha256' = encode(digest(sc.content, 'sha256'), 'hex')
      AND link.target_evidence->>'text_sha256' = encode(digest(tc.content, 'sha256'), 'hex')
  );
$$;

CREATE FUNCTION enforce_grounded_mental_link() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NOT valid_grounded_mental_link(NEW) THEN
    RAISE EXCEPTION 'mental link requires live two-sided owner-matched assertion evidence';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER grounded_mental_link_write
  BEFORE INSERT OR UPDATE ON mental_model_links
  FOR EACH ROW EXECUTE FUNCTION enforce_grounded_mental_link();

-- Similarity-only passage matches have no verified relationship label.
ALTER TABLE link_suggestions
  ADD COLUMN evidence_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE link_suggestions DROP CONSTRAINT link_suggestions_relation_check;
ALTER TABLE link_suggestions ADD CONSTRAINT link_suggestions_relation_check
  CHECK (relation IN ('unclassified', 'related_to', 'prerequisite_of', 'sub_concept_of', 'contradicts', 'extends'));
