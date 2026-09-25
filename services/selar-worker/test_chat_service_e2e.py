"""Fabricated issue #9 chat regression across authenticated API and real pgvector.

No participant material, live model call, or efficacy claim. The Go server must
reach the worker over HTTP; Postgres and its migrations are not mocked.
"""
import asyncio
import json
import os
import uuid

import asyncpg
import httpx
import pytest

from test_synthetic_service_e2e import PASSWORD, _drain_job, _request, services

PRIMARY = "# Original CedarAgent paper\nCedarAgent introduces an agent architecture for tools.\n"
COMPARISON = "# Birch comparison paper\nBirch compared a modified CedarAgent baseline on BeaconBench.\n"


@pytest.mark.timeout(120)
def test_original_source_question_abstains_through_api_worker_and_database(services, monkeypatch):
    import main

    assert os.environ.get("SELAR_SYNTHETIC_E2E") == "1"
    monkeypatch.setattr(main, "DATABASE_URL", os.environ["TEST_DATABASE_URL"])
    monkeypatch.setattr(main, "embed_text_documents", lambda texts, title="": [
        ([0.0, 1.0] if "introduces an agent architecture" in text else [1.0, 0.0])
        + [0.0] * 3070 for text in texts
    ])
    class Models:
        def generate_content(self, **_):
            return type("Result", (), {"text": json.dumps({
                "main_claim": "A fabricated research example.",
                "key_concepts": [], "assumptions": [], "open_questions": [],
                "domain": "synthetic example", "concept_edges": []})})()
    monkeypatch.setattr(main, "client", type("Client", (), {"models": Models()})())

    api, _console = services
    suffix = uuid.uuid4().hex
    with httpx.Client() as client:
        owner = _request(client, "POST", f"{api}/auth/register", expected=201,
                         json={"email": f"chat-owner-{suffix}@example.invalid", "password": PASSWORD})
        token, owner_id = owner["token"], owner["user"]["id"]
        doc_ids = []
        for title, text in (("Original CedarAgent paper", PRIMARY),
                            ("Birch comparison paper", COMPARISON)):
            doc_id = _request(client, "POST", f"{api}/api/documents/add", token, expected=202,
                              json={"source_type": "text", "title": title, "text": text})["document"]["id"]
            doc_ids.append(doc_id)
            asyncio.run(_drain_job(doc_id))
        thread = _request(client, "POST", f"{api}/api/chat/threads", token, expected=201)
        message = _request(client, "POST", f"{api}/api/chat/threads/{thread['id']}/messages",
                           token, expected=201,
                           json={"content": "Did the original CedarAgent paper use BeaconBench?"})
        assert message["role"] == "assistant"
        assert "cannot verify" in message["content"].lower()
        assert message["citations"] == []
        assert "Birch" not in message["content"] or "does not establish" in message["content"]

        async def persisted():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return await conn.fetchrow(
                    "SELECT m.user_id, m.model_version, t.query, t.candidates "
                    "FROM chat_messages m JOIN retrieval_traces t ON t.assistant_message_id=m.id "
                    "WHERE m.id=$1 AND t.user_id=$2", message["id"], owner_id)
            finally:
                await conn.close()
        row = asyncio.run(persisted())
        assert str(row["user_id"]) == owner_id
        assert row["model_version"] == "deterministic-primary-source-abstention-v1"
        assert row["query"] == "Did the original CedarAgent paper use BeaconBench?"
        assert len(json.loads(row["candidates"])) >= 2
        assert not message["citations"]  # neither source is passed off as primary proof
