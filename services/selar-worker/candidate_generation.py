"""Owner-scoped grounded candidate generation between two reading mental models.

Similarity only orders which prior readings are inspected; every persisted row
still needs an exact two-sided witness from ``grounded_overlap`` and passes the
``grounded_mental_link_write`` trigger. Generation is symmetric: a named concept
of *either* reading that the other reading states verbatim can witness a
candidate, so the result no longer depends on which PDF was ingested first.
Rows are inserted as ``candidate`` only; nothing here confirms a link.
"""
import re

from candidate_contract import grounded_overlap, persist_grounded_overlap

# Upper bound on passages read per document; the exact-name prefilter keeps the
# pairwise witness search small even for long papers.
MAX_CHUNKS_PER_DOCUMENT = 600

_ROWS_SQL = """
    SELECT c.id, c.user_id, c.document_id, c.content, c.locator, d.title, d.content_hash
    FROM chunks c JOIN documents d ON d.id = c.document_id
    WHERE c.document_id = $1 AND c.user_id = $2 AND d.user_id = $2
    ORDER BY c.chunk_index LIMIT $3
"""


def _concept_names(model):
    names = []
    for concept in model.get("key_concepts", []) or []:
        name = concept.get("name", "") if isinstance(concept, dict) else str(concept)
        if name and name not in names:
            names.append(name)
    return names


def _mentions(rows, name):
    pattern = re.compile(r"(?<!\w)" + re.escape(name) + r"(?!\w)", re.I)
    return [row for row in rows if pattern.search(row["content"] or "")]


def find_witness(model, source_rows, target_rows, owner, source_doc, target_doc):
    """First exact two-sided witness for one of ``model``'s named concepts."""
    for name in _concept_names(model):
        sources, targets = _mentions(source_rows, name), _mentions(target_rows, name)
        single = {"key_concepts": [name]}
        for source in sources:
            for target in targets:
                if grounded_overlap(single, source, target, owner, source_doc, target_doc):
                    return name, source, target
    return None


async def pair_has_link(conn, owner, model_a, model_b):
    """Any existing link (any status, either direction) blocks a new suggestion.

    This keeps one suggestion per reading pair and never re-suggests a pair the
    learner already confirmed, relabelled or rejected.
    """
    return bool(await conn.fetchval("""
        SELECT EXISTS (SELECT 1 FROM mental_model_links
                       WHERE user_id = $1 AND link_type = 'concept_overlap'
                         AND ((source_model_id = $2 AND target_model_id = $3)
                           OR (source_model_id = $3 AND target_model_id = $2)))
    """, owner, model_a, model_b))


async def link_pair(conn, owner, first, second, rows_cache=None, similarity=0.0):
    """Try ``first``→``second`` then ``second``→``first``; insert at most one candidate.

    ``first``/``second`` are dicts with ``model_id``, ``document_id`` and
    ``key_concepts``. Returns the (source document, concept) inserted, or None.
    """
    if str(first["document_id"]) == str(second["document_id"]):
        return None
    if await pair_has_link(conn, owner, first["model_id"], second["model_id"]):
        return None
    rows_cache = {} if rows_cache is None else rows_cache

    async def rows(document_id):
        key = str(document_id)
        if key not in rows_cache:
            rows_cache[key] = [dict(r) for r in await conn.fetch(
                _ROWS_SQL, document_id, owner, MAX_CHUNKS_PER_DOCUMENT)]
        return rows_cache[key]

    for source, target in ((first, second), (second, first)):
        source_rows, target_rows = await rows(source["document_id"]), await rows(target["document_id"])
        witness = find_witness(source, source_rows, target_rows, owner,
                               source["document_id"], target["document_id"])
        if not witness:
            continue
        name, source_row, target_row = witness
        if await persist_grounded_overlap(
                conn, {"key_concepts": [name]}, source_row, target_row, owner,
                source["document_id"], target["document_id"],
                source["model_id"], target["model_id"], float(similarity)):
            return str(source["document_id"]), name
    return None


async def backfill_owner(conn, owner):
    """Insert-only, idempotent pass over every pair of one owner's latest models.

    Makes no model calls and never updates or deletes existing links.
    """
    models = await conn.fetch("""
        SELECT DISTINCT ON (mm.document_id) mm.id AS model_id, mm.document_id, mm.key_concepts
        FROM document_mental_models mm JOIN documents d ON d.id = mm.document_id
        WHERE mm.user_id = $1 AND d.user_id = $1 AND mm.status = 'ready' AND d.status = 'ready'
        ORDER BY mm.document_id, mm.version DESC
    """, owner)
    models = [dict(m) for m in models]
    cache, created = {}, []
    for index, first in enumerate(models):
        for second in models[index + 1:]:
            result = await link_pair(conn, owner, first, second, cache)
            if result:
                created.append(result)
    return created
