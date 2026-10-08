"""Fabricated issue #9 feedback regressions; no participant or frozen-gold data."""
import asyncio
import os
import uuid
from types import SimpleNamespace

import asyncpg
import pytest

import main


@pytest.mark.skipif(not os.getenv("TEST_DATABASE_URL"), reason="CI pgvector database required")
def test_negatively_rated_answer_citations_do_not_boost_recency(monkeypatch):
    """An unhelpful answer's cited passage must stop being re-surfaced by recency."""
    database = os.environ["TEST_DATABASE_URL"]
    monkeypatch.setattr(main, "DATABASE_URL", database)
    # Orthogonal query vector: the passage can only come back via recency.
    monkeypatch.setattr(main, "embed_query_text", lambda _: [0.0] * 3071 + [1.0])
    monkeypatch.setattr(main, "client", SimpleNamespace(models=SimpleNamespace(
        generate_content=lambda **_: SimpleNamespace(text="Answer [S1]."))))

    async def run():
        conn = await asyncpg.connect(database)
        owner = await conn.fetchval(
            "INSERT INTO users(email,password_hash) VALUES ($1,'synthetic') RETURNING id",
            f"chat-feedback-{uuid.uuid4()}@example.invalid")
        try:
            doc = await conn.fetchval(
                "INSERT INTO documents(user_id,title,status,source_type) "
                "VALUES ($1,'Birch comparison paper','ready','pdf') RETURNING id", owner)
            chunk = await conn.fetchval(
                "INSERT INTO chunks(user_id,document_id,chunk_index,content,embedding) "
                "VALUES ($1,$2,0,'Birch ran a modified Cedar baseline.',$3::vector) RETURNING id",
                owner, doc, str([1.0] + [0.0] * 3071))
            thread = await conn.fetchval(
                "INSERT INTO chat_threads(user_id) VALUES ($1) RETURNING id", owner)
            message = await conn.fetchval(
                "INSERT INTO chat_messages(thread_id,user_id,role,content) "
                "VALUES ($1,$2,'assistant','Cedar used it [S1].') RETURNING id", thread, owner)
            await conn.execute(
                "INSERT INTO message_citations(message_id,chunk_id,rank,score,quote) "
                "VALUES ($1,$2,1,1,'q')", message, chunk)

            async def recency_ranks():
                result = await main.grounded_chat(main.ChatRequest(
                    user_id=str(owner), thread_id=str(thread), question="zzqx unrelated"))
                return [c["signals"].get("recency") for c in result["candidates"]
                        if c["chunk_id"] == str(chunk)]

            assert [r for r in await recency_ranks() if r], "baseline: recency should surface cited chunk"
            await conn.execute(
                "INSERT INTO chat_message_feedback(user_id,message_id,action) VALUES ($1,$2,'unhelpful')",
                owner, message)
            assert not [r for r in await recency_ranks() if r]
        finally:
            await conn.execute("DELETE FROM users WHERE id=$1", owner)
            await conn.close()
    asyncio.run(run())
