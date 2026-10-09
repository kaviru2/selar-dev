"""Conservative runtime evidence contract; similarity only retrieves candidates.

Reuses the frozen evaluator's exact chunk/quote/source test without importing its
annotations. Only an explicit named concept in both passages licenses overlap;
other directed relationship types need a separate assertion verifier.
"""
import hashlib
import json
import re
from uuid import UUID

from offline_evidence import _evidence


def _same_identity(left, right):
    """Compare queue string IDs with asyncpg UUIDs, never unrelated values."""
    return (isinstance(left, (str, UUID)) and isinstance(right, (str, UUID))
            and bool(str(left)) and bool(str(right)) and str(left) == str(right))


# A derived sub-term of a key concept (migration 025): lowercase, >= 8 chars.
SUBTERM_RE = re.compile(r"^[a-z][a-z0-9 -]{7,}$")


def grounded_overlap(model, source, target, owner, source_doc, target_doc, subterm=False):
    """Exact two-sided witness. ``subterm=True`` admits one derived sub-term name
    (``SUBTERM_RE``); the caller guarantees it is a whole-word part of a key concept."""
    if _same_identity(source_doc, target_doc):
        return None
    source, target = dict(source), dict(target)
    for row, expected in ((source, source_doc), (target, target_doc)):
        locator = row.get("locator")
        if isinstance(locator, str):
            try:
                locator = json.loads(locator)
            except ValueError:
                return None
        if (not _same_identity(row.get("user_id"), owner)
                or not _same_identity(row.get("document_id"), expected)
                or not row.get("id") or not row.get("content")
                or not isinstance(locator, dict) or not locator
                or not row.get("content_hash")):
            return None
        row["locator"] = locator
    concepts = model.get("key_concepts", [])
    for concept in concepts:
        name = concept.get("name", "") if isinstance(concept, dict) else str(concept)
        if subterm:
            if not SUBTERM_RE.match(name):
                continue
        elif len(name.split()) < 2 or len(name) < 12:
            continue
        passages = []
        for row in (source, target):
            sentences = re.split(r"(?<=[.!?])\s+", row["content"])
            matches = [s for s in sentences if re.search(r"(?<!\w)" + re.escape(name) + r"(?!\w)", s, re.I)]
            if len(matches) != 1 or len(matches[0]) > 800:
                break
            passages.append(matches[0])
        if len(passages) != 2:
            continue
        evidence = []
        for row, quote in zip((source, target), passages):
            doc = {"id": row["document_id"], "ref": row["document_id"],
                   "title": row.get("title", ""), "source_file": row["document_id"]}
            chunks = {row["id"]: (doc, {"id": row["id"], "text": row["content"],
                                          "locator": row["locator"]})}
            verified = _evidence(chunks, row["document_id"], {"chunk_id": row["id"], "quote": quote})
            if not verified["supported"]:
                break
            evidence.append({"chunk_id": str(row["id"]), "quote": quote,
                             "locator": row["locator"], "asserted_concept": name,
                             "asserting_source_id": str(row["document_id"]),
                             "source_snapshot_hash": row["content_hash"],
                             "text_sha256": verified["text_sha256"]})
        if len(evidence) == 2:
            return {"link_type": "concept_overlap", "concept": name,
                    "source_evidence": evidence[0], "target_evidence": evidence[1]}
    return None


async def persist_grounded_overlap(conn, model, source, target, owner, source_doc, target_doc,
                                   source_model_id, target_model_id, similarity, subterm=False):
    witness = grounded_overlap(model, source, target, owner, source_doc, target_doc, subterm)
    if not witness:
        return False
    # Recheck all witness fields against live owner-matched source/chunk rows in
    # the INSERT; the DB trigger applies the same rule to every other writer.
    result = await conn.execute("""
        INSERT INTO mental_model_links (
          user_id, source_model_id, target_model_id, link_type, similarity, confidence,
          bridge_explanation, source_evidence_chunk_id, target_evidence_chunk_id,
          source_evidence, target_evidence, status, created_via, model_version, prompt_version)
        SELECT $1, sm.id, tm.id, 'concept_overlap', $4, 0.65,
          $5, sc.id, tc.id, $6::jsonb, $7::jsonb,
          'candidate', 'ai_suggested', 'exact-shared-concept-v1', 'grounded-link-v1'
        FROM document_mental_models sm
        JOIN document_mental_models tm ON tm.id = $3 AND tm.user_id = $1
        JOIN chunks sc ON sc.id = ($6::jsonb->>'chunk_id')::uuid AND sc.user_id = $1
        JOIN chunks tc ON tc.id = ($7::jsonb->>'chunk_id')::uuid AND tc.user_id = $1
        JOIN documents sd ON sd.id = sm.document_id AND sd.user_id = $1
        JOIN documents td ON td.id = tm.document_id AND td.user_id = $1
        WHERE sm.id = $2 AND sm.user_id = $1 AND sm.status = 'ready' AND tm.status = 'ready'
          AND sc.document_id = sm.document_id AND tc.document_id = tm.document_id
          AND sd.id <> td.id
        ON CONFLICT DO NOTHING
    """, owner, source_model_id, target_model_id, similarity,
        "Both readings explicitly discuss " + witness["concept"] + ". Compare their treatment.",
        json.dumps(witness["source_evidence"]), json.dumps(witness["target_evidence"]))
    return result != "INSERT 0 0"
