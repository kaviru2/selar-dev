"""Fabricated source-attribution regressions; no participant or frozen-gold data."""
import asyncio
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
    async def connect(_):
        pytest.fail("graph command cannot select an unrelated candidate or retrieve passages")
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "embed_query_text", lambda _: pytest.fail("no embedding on graph command"))
    monkeypatch.setattr(main, "client", None)
    result = asyncio.run(main.grounded_chat(main.ChatRequest(
        user_id="fabricated-owner", thread_id="fabricated-thread", question=question)))
    assert "no graph change" in result["answer"].lower()
    assert "no preview" in result["answer"].lower()
    assert result["citations"] == [] and result["candidates"] == []
    assert result["metrics"]["model_calls"] == 0


def test_graph_action_without_relevance_evidence_does_not_link_random_candidate(monkeypatch):
    """A live candidate can be unrelated to the requested graph correction."""
    class Connection:
        async def fetchrow(self, *_):
            return {"id": uuid.UUID("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
                    "document_id": uuid.UUID("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")}
        async def close(self):
            pass
    async def connect(_):
        return Connection()
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "client", None)
    result = asyncio.run(main.grounded_chat(main.ChatRequest(
        user_id="fabricated-owner", thread_id="fabricated-thread",
        question="Update the graph based on that correction.")))
    assert "no graph change" in result["answer"].lower()
    assert "linkId=" not in result["answer"]
    assert "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" not in result["answer"]


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

def test_retrieval_collapses_normalized_duplicates_without_spending_single_document_budget():
    chunks = [
        {"chunk_id": "best", "document_id": "cedar-doc", "content": "Cedar  reports\n a result.",
         "locator": {"heading": "best locator"}, "score": 1},
        {"chunk_id": "copy", "document_id": "cedar-doc", "content": "  cedar reports a RESULT.  ",
         "locator": {"heading": "copy locator"}, "score": .9},
    ] + [
        {"chunk_id": f"distinct-{i}", "document_id": "cedar-doc",
         "content": f"Independent passage {i}.", "score": .8 - i * .1}
        for i in range(5)
    ]
    fused = main.reciprocal_rank_fusion({"vector": chunks}, limit=6)
    assert [item["chunk_id"] for item in fused] == ["best"] + [f"distinct-{i}" for i in range(5)]
    assert fused[0]["locator"] == {"heading": "best locator"}

def test_retrieval_preserves_identical_text_as_independent_evidence_in_other_document():
    chunks = [
        {"chunk_id": "first", "document_id": "cedar-doc", "content": "A reported result.",
         "locator": {"heading": "cedar"}, "score": 1},
        {"chunk_id": "same-doc-copy", "document_id": "cedar-doc", "content": "a  reported RESULT.",
         "locator": {"heading": "copy"}, "score": .9},
        {"chunk_id": "other-source", "document_id": "birch-doc", "content": "A reported result.",
         "locator": {"heading": "birch"}, "score": .8},
    ]
    fused = main.reciprocal_rank_fusion({"vector": chunks}, limit=3)
    assert [(item["chunk_id"], item["document_id"], item["locator"]) for item in fused] == [
        ("first", "cedar-doc", {"heading": "cedar"}),
        ("other-source", "birch-doc", {"heading": "birch"}),
    ]

def test_retrieval_does_not_collapse_missing_or_blank_evidence():
    chunks = [
        {"chunk_id": "no-document-1", "content": "Shared text"},
        {"chunk_id": "no-document-2", "content": "Shared text"},
        {"chunk_id": "blank-1", "document_id": "cedar-doc", "content": "   "},
        {"chunk_id": "blank-2", "document_id": "cedar-doc", "content": "\n"},
    ]
    assert {item["chunk_id"] for item in main.reciprocal_rank_fusion({"vector": chunks}, limit=4)} == {
        item["chunk_id"] for item in chunks
    }

def test_grounded_chat_citations_use_retained_chunk_and_owner_scoped_source(monkeypatch):
    rows = [
        {"chunk_id": "cedar-best", "document_id": "cedar-doc", "document_title": "Cedar report",
         "page": 4, "content": "Cedar found an effect.", "locator": {"page": 4},
         "source_type": "pdf", "score": 1},
        {"chunk_id": "cedar-copy", "document_id": "cedar-doc", "document_title": "Cedar report",
         "page": 5, "content": " cedar found an EFFECT. ", "locator": {"page": 5},
         "source_type": "pdf", "score": .9},
        {"chunk_id": "birch-independent", "document_id": "birch-doc", "document_title": "Birch report",
         "page": 7, "content": "Cedar found an effect.", "locator": {"page": 7},
         "source_type": "pdf", "score": .8},
    ]
    class Connection:
        async def fetch(self, sql, *args):
            assert args[0] == "invented-owner"
            assert "c.user_id = $1" in sql and "d.user_id = $1" in sql
            return rows if "c.embedding IS NOT NULL" in sql else []
        async def close(self):
            pass
    async def connect(_):
        return Connection()
    class Models:
        def generate_content(self, **kwargs):
            prompt = kwargs["contents"]
            assert "[S1] Cedar report, page 4" in prompt
            assert "[S2] Birch report, page 7" in prompt
            assert "page 5" not in prompt
            return SimpleNamespace(text="Two documents contain this passage [S1][S2].")
    monkeypatch.setattr(main.asyncpg, "connect", connect)
    monkeypatch.setattr(main, "embed_query_text", lambda _: [0.0] * 3072)
    monkeypatch.setattr(main, "client", SimpleNamespace(models=Models()))
    result = asyncio.run(main.grounded_chat(main.ChatRequest(
        user_id="invented-owner", thread_id="invented-thread", question="What did Cedar report?")))
    assert [(c["chunk_id"], c["document_id"], c["page"], c["locator"], c["rank"])
            for c in result["citations"]] == [
                ("cedar-best", "cedar-doc", 4, {"page": 4}, 1),
                ("birch-independent", "birch-doc", 7, {"page": 7}, 2),
            ]
    assert [c["chunk_id"] for c in result["candidates"]] == ["cedar-best", "birch-independent"]


def test_retrieval_zero_limit_returns_no_chunks():
    assert main.reciprocal_rank_fusion({"vector": [
        {"chunk_id": "one", "document_id": "one-doc", "score": 1},
    ]}, limit=0) == []


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
            chunk_ids = []
            for index, (owner, doc, text) in enumerate((
                (owners[0], comparison, "Birch compared a modified CedarAgent baseline on BeaconBench."),
                (owners[0], comparison, " birch  COMPARED a modified CedarAgent baseline on BeaconBench. "),
                (owners[0], primary, "CedarAgent introduces an agent architecture for tools."),
                (owners[1], foreign, "Foreign owner compared CedarAgent on BeaconBench."),
            )):
                chunk_ids.append(await conn.fetchval(
                    "INSERT INTO chunks(user_id,document_id,chunk_index,content,embedding) "
                    "VALUES ($1,$2,$3,$4,$5::vector) RETURNING id", owner, doc, index, text,
                    str([1.0] + [0.0] * 3071)))
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
            assert len({candidate["chunk_id"] for candidate in ordinary["candidates"]}
                       & {str(chunk_ids[0]), str(chunk_ids[1])}) == 1
            assert str(chunk_ids[3]) not in {c["chunk_id"] for c in ordinary["candidates"]}
            assert all(c["document_id"] != str(foreign) for c in ordinary["citations"])
            assert all(c["document_id"] != str(foreign) for c in disputed["citations"])
        finally:
            for owner in owners:
                await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(run())
