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

from test_synthetic_service_e2e import PASSWORD, _dismiss_optional_consent, _drain_job, _request, services

PRIMARY = "# Original CedarAgent paper\nCedarAgent introduces an agent architecture for tools.\n"
COMPARISON = "# Birch comparison paper\nBirch compared a modified CedarAgent baseline on BeaconBench.\n"


@pytest.mark.timeout(120)
def test_graph_command_does_not_search_or_pretend_to_create_preview(services):
    api, _ = services
    with httpx.Client() as client:
        owner = _request(client, "POST", f"{api}/auth/register", expected=201,
                         json={"email": f"graph-command-{uuid.uuid4().hex}@example.invalid",
                               "password": PASSWORD})
        token = owner["token"]
        thread = _request(client, "POST", f"{api}/api/chat/threads", token, expected=201)
        result = _request(client, "POST", f"{api}/api/chat/threads/{thread['id']}/messages",
                          token, expected=201, json={"content": "Update the graph based on that."})
        assert result["model_version"] == "deterministic-graph-command-boundary-v1"
        assert "no graph change" in result["content"].lower()
        assert "no preview" in result["content"].lower()
        assert result["citations"] == []
        assert not any(edge.get("created_via") == "deterministic_chat" for edge in
                       _request(client, "GET", f"{api}/api/graph", token)["edges"])
        async def trace():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return await conn.fetchrow(
                    "SELECT t.ranking_policy,t.candidates,m.values AS metrics "
                    "FROM retrieval_traces t JOIN evaluation_metrics m ON m.chat_message_id=t.assistant_message_id "
                    "WHERE t.assistant_message_id=$1 AND t.user_id=$2", result["id"], owner["user"]["id"])
            finally:
                await conn.close()
        row = asyncio.run(trace())
        assert row["ranking_policy"] == "no-research-retrieval-graph-command-v1"
        assert json.loads(row["candidates"]) == []
        assert json.loads(row["metrics"])["model_calls"] == 0


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

        # The same authenticated browser thread now asks an ordinary comparison
        # question through Next's cookie proxy, Go, worker HTTP and real SQL.
        from playwright.sync_api import sync_playwright
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(headless=True)
            context = browser.new_context()
            page = context.new_page()
            page.goto(f"{_console}/login")
            login = context.request.post(f"{_console}/api/auth/login", data={
                "email": f"chat-owner-{suffix}@example.invalid", "password": PASSWORD})
            assert login.status == 200, login.text()
            page.goto(f"{_console}/chat")
            page.locator(".chat-message.assistant").get_by_text("cannot verify", exact=False).wait_for()
            _dismiss_optional_consent(page)
            page.locator(".chat-composer textarea").fill("What did Birch compare on BeaconBench?")
            with page.expect_response(lambda response: (
                response.request.method == "POST"
                and f"/api/chat/threads/{thread['id']}/messages" in response.url
            )) as sent:
                page.locator(".chat-composer button[type=submit]").click()
            assert sent.value.status == 201
            citation = page.locator(".chat-message.assistant .chat-citations a").filter(
                has_text="Birch comparison paper")
            citation.wait_for(timeout=15000)
            assert citation.count() == 1
            assert f"docId={doc_ids[1]}" in citation.get_attribute("href")
            assert "modified CedarAgent baseline" in page.locator(".chat-message.assistant").last.inner_text()
            browser.close()

        messages = _request(client, "GET", f"{api}/api/chat/threads/{thread['id']}/messages", token)
        assistant = [item for item in messages if item["role"] == "assistant"]
        assert len(assistant) == 2 and assistant[0]["id"] == message["id"]
        comparison = assistant[1]
        assert len(comparison["citations"]) == 1
        cited = comparison["citations"][0]
        assert cited["document_id"] == doc_ids[1]
        assert cited["quote"] == "Birch compared a modified CedarAgent baseline on BeaconBench."
        assert cited["chunk_id"] and cited["rank"] >= 1
        assert "original CedarAgent used BeaconBench" not in comparison["content"]

        other = _request(client, "POST", f"{api}/auth/register", expected=201,
                         json={"email": f"chat-other-{suffix}@example.invalid", "password": PASSWORD})
        foreign = other["token"]
        assert _request(client, "GET", f"{api}/api/chat/threads", foreign) == []
        assert _request(client, "GET", f"{api}/api/chat/threads/{thread['id']}/messages", foreign) == []
        _request(client, "POST", f"{api}/api/chat/threads/{thread['id']}/messages", foreign,
                 expected=404, json={"content": "Can I read the other owner's thread?"})
        _request(client, "POST", f"{api}/api/chat/messages/{comparison['id']}/feedback", foreign,
                 expected=404, json={"action": "helpful"})

        for item, action, note in ((comparison, "helpful", ""),
                                   (message, "unhelpful", ""),
                                   (comparison, "correction", "A later baseline is not the original system.")):
            feedback = _request(client, "POST", f"{api}/api/chat/messages/{item['id']}/feedback",
                                token, expected=201,
                                json={"action": action, "correction_text": note})
            assert feedback["action"] == action
            graph = _request(client, "GET", f"{api}/api/graph", token)
            assert not any(edge.get("created_via") == "deterministic_chat" for edge in graph["edges"])

        history = _request(client, "GET", f"{api}/api/chat/threads/{thread['id']}/messages", token)
        assert next(item for item in history if item["id"] == comparison["id"])["status"] == "superseded"
        assert any(item["role"] == "system" and "Correction recorded" in item["content"] for item in history)
        assert next(item for item in history if item["id"] == comparison["id"])["graph_update"]["links_promoted"] == 0

        async def graph_state():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return await conn.fetchrow("""
                    SELECT (SELECT count(*) FROM concept_edges WHERE user_id=$1 AND created_via='deterministic_chat') AS edges,
                           (SELECT count(*) FROM adaptive_edge_evidence WHERE message_id=$2 AND active) AS evidence,
                           (SELECT count(*) FROM chat_message_feedback WHERE user_id=$1) AS feedback,
                           (SELECT COALESCE(sum(success_count),0) FROM chat_learner_projection WHERE user_id=$1) AS successes,
                           (SELECT count(*) FROM learner_concept_state WHERE user_id=$1 AND success_count>0) AS retrieval_successes
                """, owner_id, comparison["id"])
            finally:
                await conn.close()
        state = asyncio.run(graph_state())
        assert tuple(state) == (0, 0, 3, 0, 0), dict(state)
