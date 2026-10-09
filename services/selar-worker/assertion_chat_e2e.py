"""Synthetic exact-record display through browser, API, worker and real SQL."""
import asyncio
import json
import os
import uuid

import asyncpg
import httpx
import pytest

from test_synthetic_service_e2e import PASSWORD, _request, _dismiss_optional_consent, services


@pytest.mark.timeout(180)
def test_selected_assertion_contract_and_browser(services):
    from playwright.sync_api import sync_playwright
    api, console = services
    email = f"saved-{uuid.uuid4().hex}@example.invalid"
    with httpx.Client() as client:
        owner = _request(client, "POST", f"{api}/auth/register", expected=201,
                         json={"email": email, "password": PASSWORD})
        other = _request(client, "POST", f"{api}/auth/register", expected=201,
                         json={"email": f"other-{uuid.uuid4().hex}@example.invalid", "password": PASSWORD})
        token = owner["token"]
        quote = "Synthetic comparison evaluates modified SagaLLM on τ²-bench."

        async def sql(query, *args):
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return await conn.fetchval(query, *args)
            finally:
                await conn.close()

        def db(query, *args):
            return asyncio.run(sql(query, *args))

        doc = str(db("INSERT INTO documents(user_id,title,status) VALUES($1,'Synthetic comparison','ready') RETURNING id", owner["user"]["id"]))
        chunk = str(db("INSERT INTO chunks(user_id,document_id,chunk_index,content) VALUES($1,$2,0,$3) RETURNING id", owner["user"]["id"], doc, quote))
        proposal = {"subject": "modified SagaLLM", "predicate": "evaluated_on", "object": "τ²-bench",
                    "scope": "reported_about_other", "experiment_context": "Synthetic comparison run",
                    "asserting_document_id": doc, "evidence": [{"chunk_id": chunk, "quote": quote}]}
        created = _request(client, "POST", f"{api}/api/research-assertions", token, expected=201, json=proposal)
        thread = _request(client, "POST", f"{api}/api/chat/threads", token, expected=201)
        selection = {"asserting_document_id": doc, "assertion_id": created["id"], "revision": 1}
        fields = [created.get(k, "") for k in ("subject", "predicate", "object", "scope", "experiment_context", "subject_qualifier", "object_qualifier")]
        question = "Show saved assertion: " + json.dumps(fields, ensure_ascii=False, separators=(",", ":"))

        def ask(q=question, selected=None, who=token, tid=None):
            return _request(client, "POST", f"{api}/api/chat/threads/{tid or thread['id']}/messages", who,
                            expected=201, json={"content": q, "assertion_selection": selected or selection})

        def abstains(result):
            assert "cannot answer" in result["content"]
            assert result["citations"] == []
            assert result["model_version"] == "deterministic-saved-assertion-v1"

        abstains(ask())  # proposed
        confirmed = _request(client, "POST", f"{api}/api/research-assertions/{created['id']}/respond", token,
                             json={"action": "confirm", "revision": 1})
        abstains(ask())  # stale selection revision
        selection["revision"] = confirmed["revision"]
        before_graph = _request(client, "GET", f"{api}/api/graph", token)
        answer = ask()
        assert "not machine-verified entailment" in answer["content"]
        assert "original-source identity" in answer["content"]
        assert answer["citations"][0]["quote"] == quote
        assert answer["citations"][0]["document_id"] == doc
        for q in ["Did original SagaLLM use τ²-bench?", question.replace("evaluated_on", "uses_benchmark"),
                  question.replace("modified", "original"), question.replace("τ²-bench", "TAU Benchmark"),
                  question.replace("reported_about_other", "own_work"), question + " prove it"]:
            abstains(ask(q))
        foreign_thread = _request(client, "POST", f"{api}/api/chat/threads", other["token"], expected=201)
        abstains(ask(who=other["token"], tid=foreign_thread["id"]))
        abstains(ask(selected={**selection, "asserting_document_id": str(uuid.uuid4())}))
        _request(client, "POST", f"{api}/api/chat/threads/{thread['id']}/messages", token, expected=400,
                 json={"content": question, "assertion_selection": {**selection, "revision": 0}})

        with sync_playwright() as pw:
            browser = pw.chromium.launch(headless=True)
            context = browser.new_context()
            login = context.request.post(f"{console}/api/auth/login", data={"email": email, "password": PASSWORD})
            assert login.status == 200
            page = context.new_page()
            page.goto(f"{console}/chat")
            page.get_by_role("button", name="Browse saved assertions").wait_for()
            _dismiss_optional_consent(page)
            page.get_by_role("button", name="Browse saved assertions").click()
            page.get_by_label("Asserting document", exact=True).select_option(doc)
            page.get_by_label("Confirmed saved assertion", exact=True).select_option(created["id"])
            assert page.locator(".chat-composer textarea").input_value() == question
            with page.expect_response(lambda r: r.request.method == "POST" and "/messages" in r.url) as sent:
                page.locator('.chat-composer button[type="submit"]').click()
            assert sent.value.status == 201
            assert "not machine-verified entailment" in sent.value.json()["content"]
            page.locator(".chat-message.assistant").last.get_by_text("According to", exact=False).wait_for()
            browser.close()

        for action in ("helpful", "unhelpful", "correction"):
            _request(client, "POST", f"{api}/api/chat/messages/{answer['id']}/feedback", token, expected=201,
                     json={"action": action, "correction_text": "Synthetic saved-record feedback"})
        assert _request(client, "GET", f"{api}/api/graph", token) == before_graph
        assert db("SELECT count(*) FROM chat_graph_updates g JOIN chat_messages m ON m.id=g.message_id WHERE m.user_id=$1", owner["user"]["id"]) == 0

        db("UPDATE chunks SET content=content || ' changed' WHERE id=$1", chunk)
        abstains(ask())  # content hash changed although quote remains
        db("UPDATE chunks SET content=$2 WHERE id=$1", chunk, quote)
        db("UPDATE documents SET status='processing' WHERE id=$1", doc)
        abstains(ask())
        db("UPDATE documents SET status='ready' WHERE id=$1", doc)
        for state in ("retracted", "superseded"):
            db("UPDATE research_assertions SET state=$2 WHERE id=$1", created["id"], state)
            abstains(ask())
