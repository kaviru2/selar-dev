"""Fabricated source-attribution regressions; no participant or frozen-gold data."""
import asyncio
import os
import uuid
from types import SimpleNamespace

import asyncpg
import pytest

import main


@pytest.mark.parametrize("question", [
    "Did the original CedarAgent paper use BeaconBench?",
    "Did CedarAgent use BeaconBench?",
    "Did CedarAgent originally evaluate on BeaconBench?",
    "Whether the original CedarAgent system used the BeaconBench method?",
    "Was BeaconBench used in CedarAgent's original paper?",
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


def test_comparison_question_keeps_citation_bound_to_retrieved_chunk(monkeypatch):
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
        user_id="owner", thread_id="thread", question="What did Birch compare?")))
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
            request = dict(user_id=str(owners[0]), thread_id=str(uuid.uuid4()))
            disputed = await main.grounded_chat(main.ChatRequest(
                **request, question="Did the original CedarAgent paper use BeaconBench?"))
            assert not generation and disputed["citations"] == []
            assert "cannot verify" in disputed["answer"].lower()
            assert disputed["metrics"]["candidate_count"] >= 2
            ordinary = await main.grounded_chat(main.ChatRequest(
                **request, question="What did Birch compare on BeaconBench?"))
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
