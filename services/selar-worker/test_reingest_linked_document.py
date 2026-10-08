"""Issue #82: re-ingesting a document that already has grounded mental-model links.

Real worker ingestion (`process_document_task`) against migrated Postgres + pgvector;
only Gemini embeddings/generation are stubbed. All documents and accounts are invented.
"""
import asyncio
import json
import os
import uuid

import pytest

pytestmark = pytest.mark.skipif(not os.getenv("TEST_DATABASE_URL"),
                                reason="TEST_DATABASE_URL is not configured")

CONCEPT = "gradient descent optimization"
PRIOR_TEXT = "# Invented notebook\nGradient descent optimization reduces a fabricated error score in a toy orchard.\n"
NEW_TEXT = "# Invented article\nThis invented article applies gradient descent optimization to a fabricated vineyard.\n"


@pytest.fixture
def worker(monkeypatch):
    import main
    monkeypatch.setattr(main, "DATABASE_URL", os.environ["TEST_DATABASE_URL"])
    monkeypatch.setattr(main, "embed_text_documents",
                        lambda texts, title="": [[1.0] + [0.0] * 3071 for _ in texts])

    class Models:
        def generate_content(self, **_):
            return type("Result", (), {"text": json.dumps({
                "main_claim": "The invented text describes gradient descent optimization.",
                "key_concepts": [{"name": CONCEPT, "description": "A toy calculation",
                                  "evidence_chunk_index": 0}],
                "assumptions": [], "open_questions": [], "domain": "fabricated example",
                "concept_edges": []})})()

    monkeypatch.setattr(main, "client", type("Client", (), {"models": Models()})())
    return main


async def _setup(main):
    import asyncpg
    conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
    owner = await conn.fetchval(
        "INSERT INTO users(email,password_hash) VALUES ($1,'x') RETURNING id",
        f"reingest-{uuid.uuid4().hex}@example.invalid")

    async def doc(title):
        return str(await conn.fetchval(
            "INSERT INTO documents(user_id,title,status,source_type) VALUES ($1,$2,'processing','text') RETURNING id",
            owner, title))
    prior, new = await doc("Invented notebook"), await doc("Invented article")
    await main.process_document_task(prior, source_type="text", raw_text=PRIOR_TEXT, title="Invented notebook")
    await main.process_document_task(new, source_type="text", raw_text=NEW_TEXT, title="Invented article")
    link = await conn.fetchrow(
        """SELECT ml.*, valid_grounded_mental_link(ml) AS valid FROM mental_model_links ml
           WHERE ml.user_id=$1""", owner)
    assert link is not None and link["valid"] and link["status"] == "candidate"
    return conn, owner, prior, new, link


async def _confirm(conn, owner, link_id):
    # Same row/event shape as Store.RespondToMentalModelLink (Go API).
    await conn.execute(
        """UPDATE mental_model_links SET status='confirmed', created_via='user_confirmed',
               responded_at=now(), review_revision=review_revision+1 WHERE id=$1""", link_id)
    await conn.execute(
        """INSERT INTO mental_link_review_events(user_id,link_id,revision,action,before_status,after_status)
           VALUES ($1,$2,1,'confirmed','candidate','confirmed')""", owner, link_id)


@pytest.mark.parametrize("side", ["target", "source"])
def test_reingest_preserves_reviewed_link_as_frozen_snapshot(worker, side):
    main = worker

    async def exercise():
        conn, owner, prior, new, link = await _setup(main)
        try:
            await _confirm(conn, owner, link["id"])
            doc, text, title = ((prior, PRIOR_TEXT, "Invented notebook") if side == "target"
                                else (new, NEW_TEXT, "Invented article"))
            await conn.execute("UPDATE documents SET status='processing' WHERE id=$1", doc)
            await main.process_document_task(doc, source_type="text", raw_text=text, title=title)

            assert await conn.fetchval("SELECT status FROM documents WHERE id=$1", doc) == "ready"
            kept = await conn.fetchrow(
                "SELECT ml.*, valid_grounded_mental_link(ml) AS valid FROM mental_model_links ml WHERE id=$1",
                link["id"])
            # The learner's decision and the quoted evidence it was made on survive verbatim.
            assert kept is not None
            assert kept["status"] == "confirmed" and kept["review_revision"] == 1
            assert kept["created_via"] == "user_confirmed"
            assert kept["source_evidence"] == link["source_evidence"]
            assert kept["target_evidence"] == link["target_evidence"]
            assert json.loads(kept["source_evidence"])["quote"]
            # The re-ingested side's passage is gone, so the link no longer has live evidence:
            # it is hidden from the active graph/review path until re-reviewed.
            gone = "target_evidence_chunk_id" if side == "target" else "source_evidence_chunk_id"
            other = "source_evidence_chunk_id" if side == "target" else "target_evidence_chunk_id"
            assert kept[gone] is None and kept[other] == link[other]
            assert kept["valid"] is False
            assert await conn.fetchval(
                "SELECT count(*) FROM mental_link_review_events WHERE link_id=$1", link["id"]) == 1
            # Any other row is a fresh, unreviewed suggestion on live evidence; nothing
            # rewrites or reuses the reviewed row.
            others = await conn.fetch(
                """SELECT review_revision, valid_grounded_mental_link(ml) AS valid
                   FROM mental_model_links ml WHERE user_id=$1 AND id<>$2""", owner, link["id"])
            assert all(r["review_revision"] == 0 and r["valid"] for r in others)
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(exercise())


@pytest.mark.parametrize("side", ["target", "source"])
def test_reingest_replaces_unreviewed_candidate(worker, side):
    main = worker

    async def exercise():
        conn, owner, prior, new, link = await _setup(main)
        try:
            doc, text, title = ((prior, PRIOR_TEXT, "Invented notebook") if side == "target"
                                else (new, NEW_TEXT, "Invented article"))
            await conn.execute("UPDATE documents SET status='processing' WHERE id=$1", doc)
            await main.process_document_task(doc, source_type="text", raw_text=text, title=title)
            assert await conn.fetchval("SELECT status FROM documents WHERE id=$1", doc) == "ready"
            assert await conn.fetchval("SELECT count(*) FROM mental_model_links WHERE id=$1", link["id"]) == 0
            rows = await conn.fetch(
                "SELECT ml.*, valid_grounded_mental_link(ml) AS valid FROM mental_model_links ml WHERE user_id=$1",
                owner)
            # A re-ingested source re-derives a candidate only when it is the newly processed side.
            if side == "source":
                assert len(rows) == 1 and rows[0]["valid"] and rows[0]["review_revision"] == 0
            else:
                assert all(r["valid"] for r in rows)
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(exercise())


def test_reingest_keeps_review_history_of_link_rolled_back_to_candidate(worker):
    """A learner rollback to 'candidate' still has review history and must not be swept as unreviewed."""
    main = worker

    async def exercise():
        conn, owner, prior, new, link = await _setup(main)
        try:
            await _confirm(conn, owner, link["id"])
            await conn.execute(
                """UPDATE mental_model_links SET status='candidate', created_via='ai_suggested',
                       review_revision=2 WHERE id=$1""", link["id"])
            await conn.execute(
                """INSERT INTO mental_link_review_events(user_id,link_id,revision,action,before_status,after_status,target_revision)
                   VALUES ($1,$2,2,'rolled_back','confirmed','candidate',0)""", owner, link["id"])
            await conn.execute("UPDATE documents SET status='processing' WHERE id=$1", new)
            await main.process_document_task(new, source_type="text", raw_text=NEW_TEXT, title="Invented article")
            assert await conn.fetchval("SELECT status FROM documents WHERE id=$1", new) == "ready"
            kept = await conn.fetchrow("SELECT * FROM mental_model_links WHERE id=$1", link["id"])
            assert kept is not None and kept["review_revision"] == 2
            assert kept["source_evidence"] == link["source_evidence"]
            assert await conn.fetchval(
                "SELECT count(*) FROM mental_link_review_events WHERE link_id=$1", link["id"]) == 2
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(exercise())


def test_database_still_rejects_nulling_live_reviewed_evidence(worker):
    """The re-ingest exception covers only passages that no longer exist."""
    import asyncpg
    main = worker

    async def exercise():
        conn, owner, prior, new, link = await _setup(main)
        try:
            await _confirm(conn, owner, link["id"])
            with pytest.raises(asyncpg.PostgresError):
                async with conn.transaction():
                    await conn.execute(
                        "UPDATE mental_model_links SET target_evidence_chunk_id=NULL WHERE id=$1", link["id"])
            with pytest.raises(asyncpg.PostgresError):
                async with conn.transaction():
                    await conn.execute(
                        "UPDATE mental_model_links SET source_evidence_chunk_id=NULL WHERE id=$1", link["id"])
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(exercise())


def test_failed_reingest_of_linked_document_is_marked_failed_and_unindexed(worker, monkeypatch):
    main = worker

    async def exercise():
        conn, owner, prior, new, link = await _setup(main)
        try:
            await _confirm(conn, owner, link["id"])
            await conn.execute("UPDATE documents SET status='processing' WHERE id=$1", prior)

            def broken_mental_model(*_args, **_kwargs):
                raise RuntimeError("invented failure after chunks were written")
            monkeypatch.setattr(main, "deterministic_mental_model", broken_mental_model)
            with pytest.raises(RuntimeError):
                await main.process_document_task(prior, source_type="text", raw_text=PRIOR_TEXT,
                                                 title="Invented notebook")
            assert await conn.fetchval("SELECT status FROM documents WHERE id=$1", prior) == "failed"
            assert await conn.fetchval("SELECT count(*) FROM chunks WHERE document_id=$1", prior) == 0
            kept = await conn.fetchrow("SELECT * FROM mental_model_links WHERE id=$1", link["id"])
            assert kept["status"] == "confirmed" and kept["review_revision"] == 1
            assert kept["target_evidence"] == link["target_evidence"]
        finally:
            await conn.execute("DELETE FROM mental_model_links WHERE user_id=$1", owner)
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(exercise())
