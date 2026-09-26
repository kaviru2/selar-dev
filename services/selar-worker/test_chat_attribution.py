"""Fabricated source-attribution regressions; no participant or frozen-gold data."""
import asyncio
import hashlib
import json
import os
import uuid
from types import SimpleNamespace

import asyncpg
import pytest

import main


@pytest.mark.parametrize("question", [
    "Update the graph",
    "Please correct the knowledge graph based on that answer.",
    "Can you update my graph?",
])
def test_explicit_graph_action_skips_research_retrieval_and_generation(monkeypatch, question):
    calls = []
    class Connection:
        async def fetchrow(self, sql, *args):
            calls.append((sql, args))
            assert "valid_grounded_mental_link" in sql
            assert args == ("fabricated-owner",)
            return None
        async def fetch(self, *_):
            pytest.fail("graph command must not retrieve research passages")
        async def close(self):
            pass
    async def connect(_):
        return Connection()
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "embed_query_text", lambda _: pytest.fail("no embedding on graph command"))
    monkeypatch.setattr(main, "client", None)
    result = asyncio.run(main.grounded_chat(main.ChatRequest(
        user_id="fabricated-owner", thread_id="fabricated-thread", question=question)))
    assert len(calls) == 1
    assert "no graph change" in result["answer"].lower()
    assert "no preview" in result["answer"].lower()
    assert result["citations"] == [] and result["candidates"] == []
    assert result["metrics"]["model_calls"] == 0


def test_graph_action_points_only_to_existing_owner_review_candidate(monkeypatch):
    class Connection:
        async def fetchrow(self, sql, *args):
            assert args == ("fabricated-owner",)
            return {"id": uuid.UUID("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
                    "document_id": uuid.UUID("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")}
        async def close(self):
            pass
    async def connect(_):
        return Connection()
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "client", None)
    result = asyncio.run(main.grounded_chat(main.ChatRequest(
        user_id="fabricated-owner", thread_id="fabricated-thread", question="Update the graph.")))
    assert "/reader?docId=bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb&linkId=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" in result["answer"]
    assert "existing" in result["answer"].lower()
    assert "no graph change" in result["answer"].lower()
    assert result["citations"] == [] and result["candidates"] == []


@pytest.mark.parametrize("question", [
    "What does 'update the graph' mean?",
    "How did the authors update the graph in their paper?",
    "What is a knowledge graph?",
])
def test_research_questions_remain_outside_graph_command_boundary(question):
    assert not main.is_explicit_graph_command(question)


@pytest.mark.parametrize("question", [
    "Did the original CedarAgent paper use BeaconBench?",
    "Did CedarAgent use BeaconBench?",
    "Did CedarAgent originally evaluate on BeaconBench?",
    "Whether the original CedarAgent system used the BeaconBench method?",
    "Was BeaconBench used in CedarAgent's original paper?",
    "Did the original CedarAgent paper report results on BeaconBench?",
])
def test_original_source_question_abstains_without_primary_verification(monkeypatch, question):
    calls = []
    class Connection:
        async def fetch(self, sql, *args):
            calls.append((sql, args))
            # A later comparison is the only retrieved evidence; its fabricated
            # baseline result cannot establish the original publication's methods.
            if "c.embedding IS NOT NULL" in sql:
                return [{"chunk_id": "comparison-chunk", "document_id": "comparison-doc",
                         "document_title": "Birch comparison", "page": 2,
                         "content": "We ran a modified CedarAgent baseline on BeaconBench.",
                         "locator": {}, "source_type": "pdf", "score": .99}]
            return []
        async def close(self):
            pass
    async def connect(_):
        return Connection()
    class Models:
        def generate_content(self, **_):
            calls.append("generation")
            return SimpleNamespace(text="Yes, the original CedarAgent used BeaconBench [S1].")
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "embed_query_text", lambda _: [0.0] * 3072)
    monkeypatch.setattr(main, "client", SimpleNamespace(models=Models()))
    result = asyncio.run(main.grounded_chat(main.ChatRequest(
        user_id="owner", thread_id="thread", question=question)))
    assert "original" in result["answer"].lower()
    assert "not established" in result["answer"].lower() or "cannot verify" in result["answer"].lower()
    assert "Yes, the original" not in result["answer"]
    assert result["citations"] == []  # no comparison citation presented as primary proof
    assert "generation" not in calls


@pytest.mark.parametrize("question", [
    "What did Birch compare?",
    "What results did Birch report on BeaconBench?",
])
def test_comparison_question_keeps_citation_bound_to_retrieved_chunk(monkeypatch, question):
    class Connection:
        async def fetch(self, sql, *args):
            assert args[0] == "owner"
            assert "c.user_id = $1" in sql and "d.user_id = $1" in sql
            if "c.embedding IS NOT NULL" in sql:
                return [{"chunk_id": "comparison-chunk", "document_id": "comparison-doc",
                         "document_title": "Birch comparison", "page": 2,
                         "content": "We ran a modified CedarAgent baseline on BeaconBench.",
                         "locator": {}, "source_type": "pdf", "score": .99}]
            return []
        async def close(self):
            pass
    async def connect(_):
        return Connection()
    class Models:
        def generate_content(self, **kwargs):
            assert "Birch comparison" in kwargs["contents"]
            return SimpleNamespace(text="Birch reports a modified comparison baseline [S1].")
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "embed_query_text", lambda _: [0.0] * 3072)
    monkeypatch.setattr(main, "client", SimpleNamespace(models=Models()))
    result = asyncio.run(main.grounded_chat(main.ChatRequest(
        user_id="owner", thread_id="thread", question=question)))
    assert result["answer"] == "Birch reports a modified comparison baseline [S1]."
    assert [(c["chunk_id"], c["document_id"], c["rank"]) for c in result["citations"]] == [
        ("comparison-chunk", "comparison-doc", 1)]


def test_retrieval_diversifies_sources_without_promoting_comparison_to_primary():
    a = [{"chunk_id": f"a{i}", "document_id": "comparison-doc", "score": 1}
         for i in range(8)]
    b = [{"chunk_id": "b1", "document_id": "original-doc", "score": .2}]
    fused = main.reciprocal_rank_fusion({"vector": a + b, "lexical": a + b}, limit=6)
    assert "original-doc" in {item["document_id"] for item in fused}
    assert fused[0]["document_id"] == "comparison-doc"


def test_retrieval_preserves_full_budget_for_single_source_library():
    one_source = [{"chunk_id": f"only-{index}", "document_id": "only-doc", "score": 1}
                  for index in range(6)]
    fused = main.reciprocal_rank_fusion({"vector": one_source}, limit=6)
    assert [item["chunk_id"] for item in fused] == [f"only-{index}" for index in range(6)]


def test_retrieval_zero_limit_returns_no_chunks():
    assert main.reciprocal_rank_fusion({"vector": [
        {"chunk_id": "one", "document_id": "one-doc", "score": 1},
    ]}, limit=0) == []


@pytest.mark.skipif(not os.getenv("TEST_DATABASE_URL"), reason="CI pgvector database required")
def test_graph_command_only_links_live_owner_two_sided_candidate(monkeypatch):
    database = os.environ["TEST_DATABASE_URL"]
    monkeypatch.setattr(main, "DATABASE_URL", database)
    monkeypatch.setattr(main, "client", None)

    async def run():
        conn = await asyncpg.connect(database)
        owner = await conn.fetchval(
            "INSERT INTO users(email,password_hash) VALUES ($1,'synthetic') RETURNING id",
            f"graph-command-{uuid.uuid4()}@example.invalid")
        other = await conn.fetchval(
            "INSERT INTO users(email,password_hash) VALUES ($1,'synthetic') RETURNING id",
            f"graph-other-{uuid.uuid4()}@example.invalid")
        try:
            docs, chunks, models, witnesses = [], [], [], []
            concept = "gradient descent optimization"
            for index, text in enumerate((
                "The paper discusses gradient descent optimization.",
                "Gradient descent optimization is compared with momentum.",
            )):
                snapshot = f"synthetic-snapshot-{index}"
                locator = {"page": index + 1}
                doc = await conn.fetchval(
                    "INSERT INTO documents(user_id,title,content_hash,status) "
                    "VALUES ($1,$2,$3,'ready') RETURNING id", owner,
                    f"Synthetic graph reading {index}", snapshot)
                chunk = await conn.fetchval(
                    "INSERT INTO chunks(user_id,document_id,chunk_index,content,locator) "
                    "VALUES ($1,$2,0,$3,$4::jsonb) RETURNING id", owner, doc, text,
                    json.dumps(locator))
                model = await conn.fetchval(
                    "INSERT INTO document_mental_models(user_id,document_id,main_claim,key_concepts) "
                    "VALUES ($1,$2,'claim',$3) RETURNING id", owner, doc, [concept])
                docs.append(doc)
                chunks.append(chunk)
                models.append(model)
                witnesses.append(json.dumps({
                    "chunk_id": str(chunk), "asserting_source_id": str(doc),
                    "source_snapshot_hash": snapshot,
                    "text_sha256": hashlib.sha256(text.encode()).hexdigest(),
                    "quote": concept if index == 0 else "Gradient descent optimization",
                    "asserted_concept": concept, "locator": locator,
                }))
            link = await conn.fetchval(
                "INSERT INTO mental_model_links(user_id,source_model_id,target_model_id,link_type,"
                "source_evidence_chunk_id,target_evidence_chunk_id,source_evidence,target_evidence) "
                "VALUES ($1,$2,$3,'concept_overlap',$4,$5,$6::jsonb,$7::jsonb) RETURNING id",
                owner, *models, *chunks, *witnesses)
            question = "Update the graph."
            async def ask(user):
                return await main.grounded_chat(main.ChatRequest(
                    user_id=str(user), thread_id=str(uuid.uuid4()), question=question))
            owned = await ask(owner)
            assert str(link) in owned["answer"] and str(docs[0]) in owned["answer"]
            assert "no graph change" in owned["answer"].lower()
            foreign = await ask(other)
            assert str(link) not in foreign["answer"] and "no preview" in foreign["answer"].lower()
            await conn.execute("UPDATE documents SET content_hash='stale-snapshot' WHERE id=$1", docs[0])
            stale = await ask(owner)
            assert str(link) not in stale["answer"] and "no preview" in stale["answer"].lower()
            assert await conn.fetchval("SELECT count(*) FROM mental_link_review_events WHERE link_id=$1", link) == 0
        finally:
            await conn.execute("DELETE FROM users WHERE id=$1 OR id=$2", owner, other)
            await conn.close()
    asyncio.run(run())


@pytest.mark.skipif(not os.getenv("TEST_DATABASE_URL"), reason="CI pgvector database required")
def test_real_chat_retrieval_abstains_and_keeps_owner_scoped_citations(monkeypatch):
    """Actual migrated SQL/retrieval and fabricated documents; only Gemini stubbed."""
    database = os.environ["TEST_DATABASE_URL"]
    monkeypatch.setattr(main, "DATABASE_URL", database)
    monkeypatch.setattr(main, "embed_query_text", lambda _: [1.0] + [0.0] * 3071)
    generation = []
    class Models:
        def generate_content(self, **kwargs):
            generation.append(kwargs["contents"])
            return SimpleNamespace(text="The comparison reports a modified baseline [S1].")
    monkeypatch.setattr(main, "client", SimpleNamespace(models=Models()))

    async def run():
        conn = await asyncpg.connect(database)
        owners = []
        try:
            for label in ("owner", "other"):
                owner = await conn.fetchval(
                    "INSERT INTO users(email,password_hash) VALUES ($1,'synthetic') RETURNING id",
                    f"chat-attribution-{label}-{uuid.uuid4()}@example.invalid")
                owners.append(owner)
            primary = await conn.fetchval(
                "INSERT INTO documents(user_id,title,status,source_type) "
                "VALUES ($1,'Original CedarAgent paper','ready','pdf') RETURNING id", owners[0])
            comparison = await conn.fetchval(
                "INSERT INTO documents(user_id,title,status,source_type) "
                "VALUES ($1,'Birch comparison paper','ready','pdf') RETURNING id", owners[0])
            foreign = await conn.fetchval(
                "INSERT INTO documents(user_id,title,status,source_type) "
                "VALUES ($1,'Foreign benchmark paper','ready','pdf') RETURNING id", owners[1])
            for index, (owner, doc, text) in enumerate((
                (owners[0], comparison, "Birch compared a modified CedarAgent baseline on BeaconBench."),
                (owners[0], primary, "CedarAgent introduces an agent architecture for tools."),
                (owners[1], foreign, "Foreign owner compared CedarAgent on BeaconBench."),
            )):
                await conn.execute(
                    "INSERT INTO chunks(user_id,document_id,chunk_index,content,embedding) "
                    "VALUES ($1,$2,$3,$4,$5::vector)", owner, doc, index, text,
                    str([1.0] + [0.0] * 3071))
            owner_id = str(owners[0])
            thread_id = str(uuid.uuid4())
            for question in (
                "Did the original CedarAgent paper use BeaconBench?",
                "Did the original CedarAgent paper report results on BeaconBench?",
            ):
                disputed = await main.grounded_chat(main.ChatRequest(
                    user_id=owner_id, thread_id=thread_id, question=question))
                assert not generation and disputed["citations"] == []
                assert "cannot verify" in disputed["answer"].lower()
                assert disputed["metrics"]["candidate_count"] >= 2
            ordinary = await main.grounded_chat(main.ChatRequest(
                user_id=owner_id, thread_id=thread_id,
                question="What results did Birch report on BeaconBench?"))
            assert len(generation) == 1
            assert "Foreign owner" not in generation[0]
            assert "Original CedarAgent paper" in generation[0]
            assert ordinary["citations"] and ordinary["citations"][0]["document_id"] == str(comparison)
            assert all(c["document_id"] != str(foreign) for c in ordinary["citations"])
            assert all(c["document_id"] != str(foreign) for c in disputed["citations"])
        finally:
            for owner in owners:
                await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(run())
