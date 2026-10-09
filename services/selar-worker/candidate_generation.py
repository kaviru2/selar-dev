"""Owner-scoped grounded candidate generation between two reading mental models.

Similarity only orders which prior readings are inspected; every persisted row
still needs an exact two-sided witness from ``grounded_overlap`` and passes the
``grounded_mental_link_write`` trigger. Generation is symmetric: a named concept
of *either* reading that the other reading states verbatim can witness a
candidate, so the result no longer depends on which PDF was ingested first.
Rows are inserted as ``candidate`` only; nothing here confirms a link.

Up to ``MAX_LINKS_PER_PAIR`` links per reading pair, one per distinct shared
concept, chosen from both models' key-concept names plus deterministic
sub-terms of them (no extra model calls) and ranked by evidence strength.
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


# Owner decision: show at most this many concept-overlap links per reading pair.
MAX_LINKS_PER_PAIR = 5
# Derived sub-terms must be this long and occur in >= this many chunks of each reading.
MIN_SUBTERM_LENGTH = 8
MIN_SUBTERM_CHUNKS = 2
# Mentions tried per side when searching an exact two-sided witness for one term.
MAX_MENTIONS_TRIED = 25
ACTIVE_STATUSES = {"candidate", "confirmed", "relabeled"}
GENERIC_TERMS = {
    "system", "systems", "model", "models", "approach", "approaches", "method", "methods",
    "agent", "agents", "data", "paper", "papers", "result", "results", "framework",
    "frameworks", "based", "using", "analysis", "process", "processes", "problem",
    "problems", "technique", "techniques", "performance", "evaluation", "study",
    "studies", "research", "application", "applications", "approach", "concept",
    "concepts", "general", "different", "important", "multiple", "information",
    "structure", "structures", "function", "functions", "learning", "language",
    "proposed", "existing", "specific", "various", "overview", "introduction",
}


def _norm(term):
    """Near-duplicate key: case, punctuation and plural 's' are ignored."""
    words = re.findall(r"[a-z0-9]+", term.lower())
    return " ".join(w[:-1] if len(w) > 3 and w.endswith("s") else w for w in words)


def _subterms(name):
    base = re.sub(r"\([^)]*\)", " ", name.lower())
    base = re.sub(r"\s+", " ", base).strip()
    parts = {base} | set(base.split()) | set(re.split(r"[\s-]+", base))
    from candidate_contract import SUBTERM_RE
    return {p.strip(" -") for p in parts
            if len(p.strip(" -")) >= MIN_SUBTERM_LENGTH and SUBTERM_RE.match(p.strip(" -"))
            and p.strip(" -") not in GENERIC_TERMS}


def candidate_terms(first, second):
    """Deterministic shared-term candidates: lowercase term -> is an original key-concept name.

    Original names (>= 2 words, >= 12 chars, the contract's rule) of either model,
    plus sub-terms of every key concept: the name without parentheticals/acronyms,
    and its single words and hyphen parts, minus generic words and short tokens.
    """
    terms = {}
    for model in (first, second):
        for name in _concept_names(model):
            if len(name.split()) >= 2 and len(name) >= 12:
                terms[name.lower()] = True
            for sub in _subterms(name):
                terms.setdefault(sub, False)
    return terms


def rank_shared_terms(terms, rows_a, rows_b):
    """Qualifying shared terms, strongest evidence first: (term, chunks_in_a, chunks_in_b).

    Original names need one chunk on each side; derived sub-terms need
    ``MIN_SUBTERM_CHUNKS`` on each side. Order: original names, then
    min(count_a, count_b), then more words, then alphabetical.
    """
    ranked = []
    for term, original in terms.items():
        count_a, count_b = len(_mentions(rows_a, term)), len(_mentions(rows_b, term))
        need = 1 if original else MIN_SUBTERM_CHUNKS
        if count_a >= need and count_b >= need:
            ranked.append((term, count_a, count_b))
    ranked.sort(key=lambda t: (not terms[t[0]], -min(t[1], t[2]), -len(t[0].split()), t[0]))
    return ranked


def select_witnesses(first, second, first_rows, second_rows, owner, first_doc, second_doc,
                     existing=None):
    """Up to the remaining cap of grounded witnesses, one per distinct shared concept.

    ``existing`` maps already-linked concept names (either direction) to status:
    those concepts are never re-suggested and active ones count toward the cap.
    Returns ``(name, source_row, target_row, forward, subterm)`` tuples where
    ``forward`` means ``first`` is the source reading.
    """
    existing = existing or {}
    remaining = MAX_LINKS_PER_PAIR - sum(1 for st in existing.values() if st in ACTIVE_STATUSES)
    seen = {_norm(name) for name in existing if name}
    first_names = {n.lower(): n for n in _concept_names(first)}
    second_names = {n.lower(): n for n in _concept_names(second)}
    terms = candidate_terms(first, second)
    found = []
    for term, *_ in rank_shared_terms(terms, first_rows, second_rows):
        if len(found) >= remaining:
            break
        if _norm(term) in seen:
            continue
        original = terms[term]
        forward = not original or term in first_names
        name = (first_names.get(term) if forward else second_names.get(term)) if original else term
        source_rows, target_rows = (first_rows, second_rows) if forward else (second_rows, first_rows)
        source_doc, target_doc = (first_doc, second_doc) if forward else (second_doc, first_doc)
        witness = _two_sided(name, source_rows, target_rows, owner, source_doc, target_doc, not original)
        if witness:
            seen.add(_norm(term))
            found.append((name, witness[0], witness[1], forward, not original))
    return found


def _two_sided(name, source_rows, target_rows, owner, source_doc, target_doc, subterm):
    single = {"key_concepts": [name]}
    for source in _mentions(source_rows, name)[:MAX_MENTIONS_TRIED]:
        for target in _mentions(target_rows, name)[:MAX_MENTIONS_TRIED]:
            if grounded_overlap(single, source, target, owner, source_doc, target_doc, subterm):
                return source, target
    return None


async def existing_pair_concepts(conn, owner, model_a, model_b):
    """Concept name -> status for every concept_overlap link of this pair (either direction)."""
    rows = await conn.fetch("""
        SELECT coalesce(source_evidence->>'asserted_concept', '') AS concept, status
        FROM mental_model_links
        WHERE user_id = $1 AND link_type = 'concept_overlap'
          AND ((source_model_id = $2 AND target_model_id = $3)
            OR (source_model_id = $3 AND target_model_id = $2))
    """, owner, model_a, model_b)
    existing = {}
    for index, row in enumerate(rows):
        # Legacy rows without a concept still count toward the cap.
        existing[row["concept"] or f"\x00legacy-{index}"] = row["status"]
    return existing


async def link_pair(conn, owner, first, second, rows_cache=None, similarity=0.0):
    """Insert up to ``MAX_LINKS_PER_PAIR`` grounded candidates for one reading pair.

    ``first``/``second`` are dicts with ``model_id``, ``document_id`` and
    ``key_concepts``. Existing rows are never touched; a concept already linked
    with any status is never re-suggested. Returns [(source document, concept)].
    """
    if str(first["document_id"]) == str(second["document_id"]):
        return []
    # A transaction-scoped, direction-independent lock covers the read/count and
    # every insert. Concurrent ingestion and backfill must not both see vacancies.
    pair = ":".join(sorted((str(first["model_id"]), str(second["model_id"]))))
    async with conn.transaction():
        await conn.execute("SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", str(owner) + ":" + pair)
        return await _link_pair_locked(conn, owner, first, second, rows_cache, similarity)


async def _link_pair_locked(conn, owner, first, second, rows_cache, similarity):
    existing = await existing_pair_concepts(conn, owner, first["model_id"], second["model_id"])
    if sum(1 for st in existing.values() if st in ACTIVE_STATUSES) >= MAX_LINKS_PER_PAIR:
        return []
    rows_cache = {} if rows_cache is None else rows_cache

    async def rows(document_id):
        key = str(document_id)
        if key not in rows_cache:
            rows_cache[key] = [dict(r) for r in await conn.fetch(
                _ROWS_SQL, document_id, owner, MAX_CHUNKS_PER_DOCUMENT)]
        return rows_cache[key]

    first_rows, second_rows = await rows(first["document_id"]), await rows(second["document_id"])
    created = []
    for name, source_row, target_row, forward, subterm in select_witnesses(
            first, second, first_rows, second_rows, owner,
            first["document_id"], second["document_id"], existing):
        source, target = (first, second) if forward else (second, first)
        if await persist_grounded_overlap(
                conn, {"key_concepts": [name]}, source_row, target_row, owner,
                source["document_id"], target["document_id"],
                source["model_id"], target["model_id"], float(similarity), subterm):
            created.append((str(source["document_id"]), name))
    return created


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
            created.extend(await link_pair(conn, owner, first, second, cache))
    return created
