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
