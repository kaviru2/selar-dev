"""Issue #117: grounded candidate generation must not depend on ingestion order.

Unit tests use invented rows; DB tests run the real worker ingestion
(`process_document_task`) and backfill against migrated Postgres + pgvector, with
only Gemini embeddings/generation stubbed. All accounts and texts are invented.
"""
import asyncio
import json
import os
import uuid

import pytest

from candidate_generation import find_witness

OWNER = "owner"
EARLY_CONCEPT = "orchard ledger protocol"
EARLY_TEXT = ("# Invented notebook\nThe orchard ledger protocol records each fabricated harvest. "
              "It is entirely made up.\n")
LATE_TEXT = ("# Invented article\nA later invented article extends the orchard ledger protocol "
             "to a fabricated vineyard. Vineyard rotation scheduling is its own new idea.\n")


def row(doc, chunk, text, page=1):
    return {"document_id": doc, "user_id": OWNER, "id": chunk, "content": text,
            "locator": {"page": page}, "title": doc, "content_hash": "snapshot"}


def test_witness_uses_named_concept_of_the_given_side_only():
    early = [row("early", "e1", "The orchard ledger protocol records harvests.")]
    late = [row("late", "l1", "Filler sentence."), row("late", "l2", "We extend the orchard ledger protocol here.")]
    early_model = {"key_concepts": [EARLY_CONCEPT]}
    late_model = {"key_concepts": ["vineyard rotation scheduling"]}
    # The newer reading's own concepts never occur in the older one ...
    assert find_witness(late_model, late, early, OWNER, "late", "early") is None
    # ... but the older reading's named concept is stated verbatim in both.
    name, source, target = find_witness(early_model, early, late, OWNER, "early", "late")
    assert name == EARLY_CONCEPT and source["id"] == "e1" and target["id"] == "l2"


def test_witness_found_beyond_first_forty_passages():
    early = [row("early", f"e{i}", "Unrelated filler text.") for i in range(60)]
    early.append(row("early", "e60", "Only here is the orchard ledger protocol named."))
    late = [row("late", "l1", "The orchard ledger protocol is cited.")]
    found = find_witness({"key_concepts": [EARLY_CONCEPT]}, early, late, OWNER, "early", "late")
    assert found and found[1]["id"] == "e60"


def test_ambiguous_or_short_concepts_still_abstain():
    early = [row("early", "e1", "Orchard ledger protocol one. Orchard ledger protocol two.")]
    late = [row("late", "l1", "The orchard ledger protocol is cited.")]
    assert find_witness({"key_concepts": [EARLY_CONCEPT]}, early, late, OWNER, "early", "late") is None
    short = [row("early", "e1", "Ledger is used."), row("late", "l1", "Ledger is used.")]
    assert find_witness({"key_concepts": ["Ledger"]}, short[:1], short[1:], OWNER, "early", "late") is None


def test_foreign_owner_rows_never_witness():
    early = [row("early", "e1", "The orchard ledger protocol records harvests.")]
    foreign = [{**row("late", "l1", "The orchard ledger protocol is cited."), "user_id": "intruder"}]
    assert find_witness({"key_concepts": [EARLY_CONCEPT]}, early, foreign, OWNER, "early", "late") is None


db = pytest.mark.skipif(not os.getenv("TEST_DATABASE_URL"), reason="TEST_DATABASE_URL is not configured")


@pytest.fixture
def worker(monkeypatch):
    import main
    monkeypatch.setattr(main, "DATABASE_URL", os.environ["TEST_DATABASE_URL"])
    monkeypatch.setattr(main, "embed_text_documents",
                        lambda texts, title="": [[1.0] + [0.0] * 3071 for _ in texts])

    class Models:
        def generate_content(self, contents, **_):
            # Five named concepts each, so the deterministic padder adds none.
            names = ([EARLY_CONCEPT, "harvest audit trail", "fabricated orchard yield",
                      "toy grading rubric", "invented crate tally"]
                     if "records each fabricated harvest" in contents else
                     ["vineyard rotation scheduling", "imaginary trellis spacing",
                      "fabricated grape census", "toy irrigation budget", "invented barrel ageing"])
            return type("Result", (), {"text": json.dumps({
                "main_claim": "The invented text describes a fabricated procedure.",
                "key_concepts": [{"name": n, "description": "toy", "evidence_chunk_index": 0} for n in names],
                "assumptions": [], "open_questions": [], "domain": "fabricated example",
                "concept_edges": []})})()

    monkeypatch.setattr(main, "client", type("Client", (), {"models": Models()})())
    return main


async def _owner_with_docs(conn):
    owner = await conn.fetchval("INSERT INTO users(email,password_hash) VALUES ($1,'x') RETURNING id",
                                f"order-{uuid.uuid4().hex}@example.invalid")

    async def doc(title):
        return str(await conn.fetchval(
            "INSERT INTO documents(user_id,title,status,source_type) VALUES ($1,$2,'processing','text') RETURNING id",
            owner, title))
    return owner, await doc("Invented notebook"), await doc("Invented article")


async def _links(conn, owner):
    return await conn.fetch("""
        SELECT ml.*, valid_grounded_mental_link(ml) AS valid, sm.document_id AS source_doc
        FROM mental_model_links ml JOIN document_mental_models sm ON sm.id = ml.source_model_id
        WHERE ml.user_id = $1""", owner)


@db
def test_newer_reading_links_through_older_readings_named_concept(worker):
    """RED before #117: only the newer reading's concepts were tried, so no candidate."""
    import asyncpg

    async def exercise():
        conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
        owner, early, late = await _owner_with_docs(conn)
        try:
            await worker.process_document_task(early, source_type="text", raw_text=EARLY_TEXT, title="Invented notebook")
            await worker.process_document_task(late, source_type="text", raw_text=LATE_TEXT, title="Invented article")
            links = await _links(conn, owner)
            assert len(links) == 1
            link = links[0]
            assert link["valid"] and link["status"] == "candidate" and link["review_revision"] == 0
            assert link["created_via"] == "ai_suggested" and link["link_type"] == "concept_overlap"
            assert str(link["source_doc"]) == early
            source, target = json.loads(link["source_evidence"]), json.loads(link["target_evidence"])
            assert source["asserted_concept"] == target["asserted_concept"] == EARLY_CONCEPT
            assert source["asserting_source_id"] == early and target["asserting_source_id"] == late
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(exercise())


@db
def test_backfill_is_insert_only_idempotent_owner_scoped_and_respects_decisions(worker):
    import asyncpg
    from candidate_generation import backfill_owner

    async def exercise():
        conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
        owner, early, late = await _owner_with_docs(conn)
        other, other_early, other_late = await _owner_with_docs(conn)
        try:
            for user_docs in ((early, late), (other_early, other_late)):
                await worker.process_document_task(user_docs[0], source_type="text", raw_text=EARLY_TEXT, title="Invented notebook")
                await worker.process_document_task(user_docs[1], source_type="text", raw_text=LATE_TEXT, title="Invented article")
            # Simulate the pre-#117 state: no candidate for this owner.
            await conn.execute("DELETE FROM mental_model_links WHERE user_id = ANY($1::uuid[])", [owner, other])
            created = await backfill_owner(conn, owner)
            assert created == [(early, EARLY_CONCEPT)]
            assert await backfill_owner(conn, owner) == []  # idempotent
            assert len(await _links(conn, owner)) == 1
            assert await _links(conn, other) == []  # another owner's library is untouched
            # A learner rejection is final: backfill never re-suggests the pair.
            link_id = (await _links(conn, owner))[0]["id"]
            await conn.execute("""UPDATE mental_model_links SET status='rejected', responded_at=now(),
                                  review_revision=1 WHERE id=$1""", link_id)
            assert await backfill_owner(conn, owner) == []
            rows = await _links(conn, owner)
            assert len(rows) == 1 and rows[0]["status"] == "rejected"
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id = ANY($1::uuid[])", [owner, other])
            await conn.execute("DELETE FROM users WHERE id = ANY($1::uuid[])", [owner, other])
            await conn.close()
    asyncio.run(exercise())


@db
def test_backfill_cli_dry_run_rolls_back(worker, monkeypatch):
    import asyncpg
    import backfill_candidates
    monkeypatch.setenv("DATABASE_URL", os.environ["TEST_DATABASE_URL"])

    async def exercise():
        conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
        owner, early, late = await _owner_with_docs(conn)
        count = "SELECT count(*) FROM mental_model_links WHERE user_id=$1"
        try:
            await worker.process_document_task(early, source_type="text", raw_text=EARLY_TEXT, title="Invented notebook")
            await worker.process_document_task(late, source_type="text", raw_text=LATE_TEXT, title="Invented article")
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            assert await backfill_candidates.run(str(owner), dry_run=True) == [(early, EARLY_CONCEPT)]
            assert await conn.fetchval(count, owner) == 0
            assert await backfill_candidates.run(str(owner), dry_run=False) == [(early, EARLY_CONCEPT)]
            assert await conn.fetchval(count, owner) == 1
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(exercise())


# ── Up to MAX_LINKS_PER_PAIR grounded concept links per reading pair ──

from candidate_generation import MAX_LINKS_PER_PAIR, candidate_terms, rank_shared_terms, select_witnesses


def test_cap_is_five():
    assert MAX_LINKS_PER_PAIR == 5


def test_candidate_terms_union_names_and_derived_subterms():
    first = {"key_concepts": [{"name": "Compensation-based recovery"}, "Saga Pattern"]}
    second = {"key_concepts": ["Retry-Aware Compensation (RAC)", "Agent System Model"]}
    terms = candidate_terms(first, second)
    assert terms["compensation-based recovery"] is True and terms["saga pattern"] is True
    assert terms["retry-aware compensation (rac)"] is True
    # Derived sub-terms: parentheticals/acronyms stripped, generic and short words dropped.
    assert terms["compensation"] is False and terms["recovery"] is False
    assert terms["retry-aware compensation"] is False
    assert terms["agent system model"] is True
    for generic in ("system", "agent", "model", "rac", "saga", "pattern", "based"):
        assert generic not in terms


def text_rows(doc, sentences):
    return [row(doc, f"{doc}{i}", s) for i, s in enumerate(sentences)]


def test_derived_subterm_needs_two_chunks_in_each_document():
    a = text_rows("a", ["Compensation undoes a step.", "Compensation is logged.", "Unrelated."])
    b = text_rows("b", ["Compensation is rare here.", "Unrelated text."])
    terms = {"compensation": False}
    assert rank_shared_terms(terms, a, b) == []
    b.append(row("b", "b9", "Compensation again appears."))
    assert [t for t, *_ in rank_shared_terms(terms, a, b)] == ["compensation"]


def test_ranking_prefers_original_names_then_evidence_then_multiword():
    a = text_rows("a", ["The orchard ledger protocol is old.", "Recovery happens.", "Recovery again.",
                        "Recovery thrice.", "Compensation once.", "Compensation twice."])
    b = text_rows("b", ["We cite the orchard ledger protocol.", "Recovery here.", "Recovery there.",
                        "Recovery everywhere.", "Compensation one.", "Compensation two."])
    terms = {"orchard ledger protocol": True, "compensation": False, "recovery": False}
    assert [t for t, *_ in rank_shared_terms(terms, a, b)] == ["orchard ledger protocol", "recovery", "compensation"]


def _shared(n):
    names = [f"fabricated concept {chr(97 + i)}lpha" for i in range(n)]
    a = text_rows("a", [f"Here the {name} is defined." for name in names])
    b = text_rows("b", [f"Later the {name} is reused." for name in names])
    return names, a, b


def test_select_witnesses_caps_and_skips_existing_concepts():
    names, a, b = _shared(8)
    model = {"key_concepts": names}
    found = select_witnesses(model, {"key_concepts": []}, a, b, OWNER, "a", "b")
    assert len(found) == MAX_LINKS_PER_PAIR and len({n for n, *_ in found}) == MAX_LINKS_PER_PAIR
    # Existing active links count toward the cap; reviewed concepts are never re-suggested.
    existing = {names[0].upper(): "rejected", names[1]: "confirmed", names[2]: "candidate"}
    again = select_witnesses(model, {"key_concepts": []}, a, b, OWNER, "a", "b", existing)
    concepts = [n for n, *_ in again]
    assert len(concepts) == MAX_LINKS_PER_PAIR - 2
    assert not {names[0], names[1], names[2]} & set(concepts)


def test_select_witnesses_dedupes_near_identical_names():
    a = [row("a", "a0", "The saga pattern coordinates steps."), row("a", "a1", "We call it the Saga Patterns idea.")]
    b = [row("b", "b0", "A saga pattern is used."), row("b", "b1", "Saga Patterns are classic.")]
    model = {"key_concepts": ["Saga Pattern", "Saga Patterns"]}
    found = select_witnesses(model, {"key_concepts": []}, a, b, OWNER, "a", "b")
    assert [n.lower() for n, *_ in found] == ["saga pattern"]


def test_derived_subterm_witness_is_grounded_in_both_quotes():
    a = text_rows("a", ["Compensation undoes a failed step.", "Compensation is logged twice."])
    b = text_rows("b", ["Our compensation differs.", "Compensation runs at the end."])
    first = {"key_concepts": ["Compensation-based recovery"]}
    second = {"key_concepts": ["Retry-Aware Compensation (RAC)"]}
    found = select_witnesses(first, second, a, b, OWNER, "a", "b")
    assert [n for n, *_ in found] == ["compensation"]
    _, source, target, forward, subterm = found[0]
    assert forward and subterm
    assert "compensation" in source["content"].lower() and "compensation" in target["content"].lower()


MULTI = ["orchard ledger protocol", "harvest audit trail", "fabricated orchard yield",
         "toy grading rubric", "invented crate tally", "imaginary trellis spacing", "pretend pruning cadence"]
MULTI_A = "# Invented A\n" + " ".join(f"Notebook A defines the {n} carefully." for n in MULTI) + "\n"
MULTI_B = "# Invented B\n" + " ".join(f"Article B reuses the {n} later." for n in MULTI) + "\n"


@pytest.fixture
def multi_worker(monkeypatch):
    import main
    monkeypatch.setattr(main, "DATABASE_URL", os.environ["TEST_DATABASE_URL"])
    monkeypatch.setattr(main, "embed_text_documents",
                        lambda texts, title="": [[1.0] + [0.0] * 3071 for _ in texts])

    class Models:
        def generate_content(self, contents, **_):
            return type("Result", (), {"text": json.dumps({
                "main_claim": "The invented text describes a fabricated procedure.",
                "key_concepts": [{"name": n, "description": "toy", "evidence_chunk_index": 0} for n in MULTI],
                "assumptions": [], "open_questions": [], "domain": "fabricated example",
                "concept_edges": []})})()

    monkeypatch.setattr(main, "client", type("Client", (), {"models": Models()})())
    return main


@db
def test_multiple_grounded_links_per_pair_capped_and_rejections_respected(multi_worker):
    import asyncpg
    from candidate_generation import backfill_owner

    async def exercise():
        conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
        owner, early, late = await _owner_with_docs(conn)
        try:
            await multi_worker.process_document_task(early, source_type="text", raw_text=MULTI_A, title="Invented A")
            await multi_worker.process_document_task(late, source_type="text", raw_text=MULTI_B, title="Invented B")
            links = await _links(conn, owner)
            assert len(links) == MAX_LINKS_PER_PAIR
            assert all(l["valid"] and l["status"] == "candidate" for l in links)
            concepts = [json.loads(l["source_evidence"])["asserted_concept"].lower() for l in links]
            assert len(set(concepts)) == MAX_LINKS_PER_PAIR
            assert all(c in l["bridge_explanation"].lower() for c, l in zip(concepts, links))
            # Reject one, drop the other candidates, backfill: never re-suggest the rejected concept.
            rejected = links[0]
            await conn.execute("""UPDATE mental_model_links SET status='rejected', responded_at=now(),
                                  review_revision=1 WHERE id=$1""", rejected["id"])
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1 AND status='candidate'", owner)
            created = await backfill_owner(conn, owner)
            assert len(created) == MAX_LINKS_PER_PAIR
            assert concepts[0] not in {c.lower() for _, c in created}
            assert await backfill_owner(conn, owner) == []  # idempotent, cap reached
            rows = await _links(conn, owner)
            assert len(rows) == MAX_LINKS_PER_PAIR + 1
            assert [r["status"] for r in rows].count("rejected") == 1
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(exercise())
