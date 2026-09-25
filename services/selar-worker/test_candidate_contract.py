"""Synthetic assertions at the ingestion boundary (not frozen benchmark annotations)."""
import asyncio
import json
import os
import pytest

from candidate_contract import grounded_overlap, persist_grounded_overlap


def row(doc, user, chunk, text, locator=None):
    return {"document_id": doc, "user_id": user, "id": chunk, "content": text,
            "locator": locator or {"page": 2}, "title": doc, "content_hash": "snapshot"}


NEW = row("new", "owner", "n1", "The paper discusses gradient descent optimization.")
PRIOR = row("prior", "owner", "p1", "Gradient descent optimization is compared with momentum.")
CONCEPT = {"key_concepts": [{"name": "gradient descent optimization"}]}


def test_exact_two_sided_owner_matched_overlap_keeps_a_candidate():
    link = grounded_overlap(CONCEPT, NEW, PRIOR, "owner", "new", "prior")
    assert link["link_type"] == "concept_overlap"
    assert link["source_evidence"]["quote"] in NEW["content"]
    assert link["target_evidence"]["quote"] in PRIOR["content"]
    assert link["source_evidence"]["asserting_source_id"] == "new"
    assert link["source_evidence"]["asserted_concept"] == "gradient descent optimization"
    assert link["target_evidence"]["asserting_source_id"] == "prior"
    assert link["source_evidence"]["locator"] == {"page": 2}


def test_json_locator_from_asyncpg_is_preserved():
    assert grounded_overlap(CONCEPT, {**NEW, "locator": '{"page":2}'}, PRIOR,
                            "owner", "new", "prior")["source_evidence"]["locator"] == {"page": 2}


@pytest.mark.parametrize("change", [
    {"user_id": "intruder"}, {"document_id": "other"}, {"content": "no support"},
    {"locator": {}}, {"content_hash": ""},
])
def test_wrong_owner_document_quote_locator_or_snapshot_abstains(change):
    assert grounded_overlap(CONCEPT, NEW, {**PRIOR, **change}, "owner", "new", "prior") is None


def test_direction_cannot_be_swapped():
    assert grounded_overlap(CONCEPT, NEW, PRIOR, "owner", "prior", "new") is None


def test_similarity_without_shared_asserted_concept_abstains():
    assert grounded_overlap({"key_concepts": [{"name": "quantum gravity"}]}, NEW, PRIOR,
                            "owner", "new", "prior") is None


class RecordingConnection:
    def __init__(self):
        self.statements = []

    async def execute(self, sql, *args):
        self.statements.append((sql, args))
        return "INSERT 0 1"


def test_persistence_requires_verified_pair_before_sql():
    async def exercise():
        conn = RecordingConnection()
        assert not await persist_grounded_overlap(conn, CONCEPT, NEW, {**PRIOR, "user_id": "intruder"},
                                                    "owner", "new", "prior", "model-new", "model-prior", .93)
        assert conn.statements == []
        assert await persist_grounded_overlap(conn, CONCEPT, NEW, PRIOR, "owner", "new", "prior",
                                              "model-new", "model-prior", .93)
        sql, args = conn.statements[0]
        assert "INSERT INTO mental_model_links" in sql
        assert "source_evidence" in sql and "target_evidence" in sql
        assert args[1:3] == ("model-new", "model-prior")
    asyncio.run(exercise())


def test_upload_worker_does_not_write_unverified_concept_edges():
    import inspect
    from main import process_document_task
    source = inspect.getsource(process_document_task)
    assert "INSERT INTO concept_edges" not in source


def test_real_database_rejects_forged_and_stale_witnesses():
    if not os.getenv("TEST_DATABASE_URL"):
        pytest.skip("No live test Postgres/pgvector; CI exercises service boundary")
    import asyncpg
    from hashlib import sha256

    async def exercise():
        conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
        tx = conn.transaction()
        await tx.start()
        try:
            owner = await conn.fetchval("INSERT INTO users(email,password_hash) VALUES ('contract-owner@example.invalid','x') RETURNING id")
            intruder = await conn.fetchval("INSERT INTO users(email,password_hash) VALUES ('contract-intruder@example.invalid','x') RETURNING id")
            async def doc(user, title):
                return await conn.fetchval("INSERT INTO documents(user_id,title,content_hash,status) VALUES ($1,$2,'snapshot','ready') RETURNING id", user, title)
            new_doc, prior_doc, foreign_doc = await doc(owner, "new"), await doc(owner, "prior"), await doc(intruder, "foreign")
            async def chunk(user, document, text):
                return await conn.fetchval("INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) VALUES ($1,$2,0,$3,'{\"page\":2}') RETURNING id", user, document, text)
            new_chunk = await chunk(owner, new_doc, NEW["content"])
            prior_chunk = await chunk(owner, prior_doc, PRIOR["content"])
            foreign_chunk = await chunk(intruder, foreign_doc, PRIOR["content"])
            async def mental(document):
                return await conn.fetchval("INSERT INTO document_mental_models(user_id,document_id,main_claim,key_concepts) VALUES ($1,$2,'claim',ARRAY['gradient descent optimization']) RETURNING id", owner, document)
            new_model, prior_model = await mental(new_doc), await mental(prior_doc)
            source = {**NEW, "user_id": owner, "document_id": new_doc, "id": new_chunk}
            target = {**PRIOR, "user_id": owner, "document_id": prior_doc, "id": prior_chunk}
            with pytest.raises(asyncpg.PostgresError):
                async with conn.transaction():
                    await conn.execute("INSERT INTO mental_model_links(user_id,source_model_id,target_model_id,link_type,similarity) VALUES ($1,$2,$3,'concept_overlap',.99)", owner, new_model, prior_model)
            assert await persist_grounded_overlap(conn, CONCEPT, source, target, owner, new_doc,
                                                   prior_doc, new_model, prior_model, .91)
            link = await conn.fetchrow("SELECT ml.*, valid_grounded_mental_link(ml) AS valid FROM mental_model_links ml WHERE source_model_id=$1", new_model)
            assert link["valid"] and link["link_type"] == "concept_overlap"
            for field, value in (("quote", "invented"), ("asserting_source_id", str(new_doc)),
                                 ("chunk_id", str(foreign_chunk)), ("source_snapshot_hash", "stale"),
                                 ("locator", {"page": 99}), ("asserted_concept", "invented overlap")):
                forged = {**json.loads(link["target_evidence"]), field: value}
                with pytest.raises(asyncpg.PostgresError):
                    async with conn.transaction():
                        await conn.execute("UPDATE mental_model_links SET target_evidence=$1 WHERE id=$2", json.dumps(forged), link["id"])
            with pytest.raises(asyncpg.PostgresError):
                async with conn.transaction():
                    await conn.execute("UPDATE mental_model_links SET source_model_id=$1, target_model_id=$2 WHERE id=$3",
                                       prior_model, new_model, link["id"])
            # A changed source snapshot invalidates display/confirmation even if old text survives.
            await conn.execute("UPDATE documents SET content_hash='refreshed' WHERE id=$1", prior_doc)
            assert not await conn.fetchval("SELECT valid_grounded_mental_link(ml) FROM mental_model_links ml WHERE id=$1", link["id"])
        finally:
            await tx.rollback()
            await conn.close()
    asyncio.run(exercise())
