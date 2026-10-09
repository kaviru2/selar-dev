-- Up to five grounded concept-overlap links per reading pair (one per distinct
-- shared concept) instead of one per directed model pair.
--
-- 1. The old UNIQUE (source_model_id, target_model_id, link_type) allowed only one
--    row per direction; uniqueness is now per asserted concept (case-insensitive).
-- 2. Besides a model's own multi-word key concept (unchanged rule), a candidate may
--    assert a deterministic sub-term of a key concept of either reading: lowercase,
--    >= 8 characters, made of [a-z0-9 -], occurring as whole words inside one of the
--    two models' key concepts. Both quotes must still state it verbatim and every
--    other owner/document/chunk/snapshot/hash check is unchanged.
DO $$
DECLARE c text;
BEGIN
  FOR c IN SELECT conname FROM pg_constraint
           WHERE conrelid = 'mental_model_links'::regclass AND contype = 'u'
  LOOP
    EXECUTE format('ALTER TABLE mental_model_links DROP CONSTRAINT %I', c);
  END LOOP;
END $$;

CREATE UNIQUE INDEX mental_model_links_pair_concept_key ON mental_model_links
  (source_model_id, target_model_id, link_type, lower(source_evidence->>'asserted_concept'));

CREATE OR REPLACE FUNCTION valid_grounded_mental_link(link mental_model_links) RETURNS boolean
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
      AND lower(link.source_evidence->>'asserted_concept') = lower(link.target_evidence->>'asserted_concept')
      AND (
        (length(coalesce(link.source_evidence->>'asserted_concept', '')) >= 12
         AND position(' ' IN link.source_evidence->>'asserted_concept') > 0
         AND EXISTS (SELECT 1 FROM unnest(sm.key_concepts) concept
                     WHERE lower(concept) = lower(link.source_evidence->>'asserted_concept')))
        OR
        (coalesce(link.source_evidence->>'asserted_concept', '') ~ '^[a-z][a-z0-9 -]{7,}$'
         AND EXISTS (SELECT 1 FROM unnest(sm.key_concepts || tm.key_concepts) concept
                     WHERE lower(concept) ~ ('(^|[^a-z0-9])' || (link.source_evidence->>'asserted_concept') || '([^a-z0-9]|$)')))
      )
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
