"""Real Go API -> durable queue -> Python worker -> PostgreSQL -> Next/Chromium.

Only Gemini embeddings/generation are replaced. Requires a throwaway migrated
pgvector database, TEST_API_BINARY and TEST_CONSOLE_DIR; never point at user data.
"""
import asyncio
import json
import os
from pathlib import Path
import socket
import subprocess
import time
import uuid

import asyncpg
import httpx
import pytest

# These are fabricated, not study participants or frozen benchmark examples.
PASSWORD = "synthetic-only-password-2026"
PRIOR = "# Notebook A\nGradient descent optimization reduces a fabricated error score in a toy garden.\n"
NEW = "# Notebook B\nGradient descent optimization reduces a fabricated error score in a toy workshop.\n"
NEGATIVE = "# Notebook C\nA paper boat floats in a fabricated pond without an optimization claim.\n"


def _port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def _ready(url, process, deadline=60):
    with httpx.Client() as client:
        until = time.monotonic() + deadline
        while time.monotonic() < until:
            if process.poll() is not None:
                raise AssertionError(f"service exited early: {process.returncode}")
            try:
                if client.get(url, timeout=2).status_code == 200:
                    return
            except httpx.TransportError:
                pass
            time.sleep(.3)
    raise AssertionError(f"service did not become ready: {url}")


@pytest.fixture(scope="module")
def services():
    db = os.environ.get("TEST_DATABASE_URL")
    binary = os.environ.get("TEST_API_BINARY")
    console = os.environ.get("TEST_CONSOLE_DIR")
    if os.environ.get("SELAR_SYNTHETIC_E2E") != "1":
        pytest.skip("dedicated pgvector/API/Next/Chromium job runs this service E2E")
    if not all((db, binary, console)):
        pytest.fail("real synthetic E2E requires TEST_DATABASE_URL, TEST_API_BINARY, TEST_CONSOLE_DIR")
    assert "selar_e2e" in db, "only use the isolated CI database"
    api_port, next_port = _port(), _port()
    api = subprocess.Popen([binary], env={**os.environ, "DATABASE_URL": db,
        "PORT": str(api_port), "JWT_SECRET": "ci-synthetic-jwt-not-production", "APP_ENV": "development"})
    next_server = None
    try:
        _ready(f"http://127.0.0.1:{api_port}/healthz", api)
        next_server = subprocess.Popen(["pnpm", "start", "-p", str(next_port)], cwd=console,
            env={**os.environ, "API_INTERNAL_URL": f"http://127.0.0.1:{api_port}"})
        _ready(f"http://127.0.0.1:{next_port}/login", next_server)
        yield f"http://127.0.0.1:{api_port}", f"http://127.0.0.1:{next_port}"
    finally:
        for process in (next_server, api):
            if process:
                process.terminate()
                try:
                    process.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()


def _request(client, method, url, token=None, expected=200, **kwargs):
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    response = client.request(method, url, headers=headers, timeout=30, **kwargs)
    assert response.status_code == expected, f"{method} {url}: {response.status_code} {response.text[:500]}"
    return response.json()


async def _drain_job(document_id):
    import main
    job = await main.claim_ingestion_job()
    assert job is not None and str(job["document_id"]) == document_id
    await main.run_claimed_ingestion_job(job)
    conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
    try:
        doc = await conn.fetchrow("SELECT status, content_hash FROM documents WHERE id=$1", document_id)
        state = await conn.fetchval("SELECT status FROM ingestion_jobs WHERE document_id=$1", document_id)
        assert doc["status"] == "ready" and doc["content_hash"] and state == "completed", (dict(doc), state)
    finally:
        await conn.close()


@pytest.mark.timeout(180)
def test_auth_ingestion_exact_witness_review_graph_reader_and_stale_rejection(services, monkeypatch):
    import main
    from playwright.sync_api import sync_playwright

    api, console = services
    # No external model/network access; all indexing, linking, persistence and
    # review logic below runs unchanged. Deliberately deterministic 3072-D vectors.
    def embedding(contents, title=""):
        return [[1.0] + [0.0] * 3071 for _ in contents]
    class Models:
        def generate_content(self, **_):
            return type("Result", (), {"text": json.dumps({
                "main_claim": "The fabricated readings describe gradient descent optimization.",
                "key_concepts": [{"name": "gradient descent optimization",
                                  "description": "A fabricated optimization example", "evidence_chunk_index": 0}],
                "assumptions": [], "open_questions": [], "domain": "synthetic example",
                "concept_edges": []})})()
    monkeypatch.setattr(main, "embed_text_documents", embedding)
    monkeypatch.setattr(main, "client", type("Client", (), {"models": Models()})())
    monkeypatch.setattr(main, "DATABASE_URL", os.environ["TEST_DATABASE_URL"])

    suffix = uuid.uuid4().hex
    with httpx.Client() as client:
        users = []
        for label in ("owner", "other"):
            payload = _request(client, "POST", f"{api}/auth/register", expected=201,
                json={"email": f"{label}-{suffix}@example.invalid", "password": PASSWORD})
            assert payload["user"]["id"] and payload["token"]
            users.append(payload)
        owner, other = users
        token, foreign = owner["token"], other["token"]
        docs = []
        for title, text in (("Fabricated notebook A", PRIOR), ("Fabricated notebook B", NEW)):
            result = _request(client, "POST", f"{api}/api/documents/add", token, expected=202,
                json={"source_type": "text", "title": title, "text": text})
            assert result["document"]["status"] == "processing"
            docs.append(result["document"]["id"])
            asyncio.run(_drain_job(docs[-1]))
        prior, latest = docs
        assert _request(client, "GET", f"{api}/api/documents/{latest}/content", token)["document"]["status"] == "ready"
        _request(client, "GET", f"{api}/api/documents/{latest}/content", foreign, expected=404)
        _request(client, "GET", f"{api}/api/documents/{latest}/mental-model", foreign, expected=404)
        links = _request(client, "GET", f"{api}/api/mental-model-links?document_id={latest}", token)
        assert len(links) == 1, links
        link = links[0]
        assert link["status"] == "candidate" and link["link_type"] == "concept_overlap"
        assert link["source_document_id"] == latest and link["target_document_id"] == prior
        for side, doc_id in (("source", latest), ("target", prior)):
            evidence = json.loads(link[f"{side}_evidence"])
            assert evidence["asserting_source_id"] == doc_id
            assert evidence["quote"] == link[f"{side}_quote"]
            assert "gradient descent optimization" in evidence["quote"].lower()
            assert evidence["source_snapshot_hash"] and evidence["chunk_id"]
        assert _request(client, "GET", f"{api}/api/mental-model-links", foreign) == []
        _request(client, "POST", f"{api}/api/mental-model-links/{link['id']}/respond", foreign,
                 expected=404, json={"action": "confirmed"})
        assert not any(edge["id"] == link["id"] for edge in _request(client, "GET", f"{api}/api/graph", foreign)["edges"])
        # A similarity-only near negative may be retrieved; it must not become a
        # grounded candidate without a two-sided exact assertion.
        neg = _request(client, "POST", f"{api}/api/documents/add", token, expected=202,
            json={"source_type": "text", "title": "Fabricated notebook C", "text": NEGATIVE})["document"]["id"]
        asyncio.run(_drain_job(neg))
        assert _request(client, "GET", f"{api}/api/mental-model-links?document_id={neg}", token) == []

        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(headless=True)
            context = browser.new_context()
            page = context.new_page()
            page.goto(f"{console}/login")
            login = context.request.post(f"{console}/api/auth/login", data={
                "email": f"owner-{suffix}@example.invalid", "password": PASSWORD})
            assert login.status == 200, login.text()
            page.goto(f"{console}/reader?docId={latest}")
            card = page.locator(".mental-link-card").filter(has_text="concept overlap")
            card.wait_for(timeout=20000)
            assert card.count() == 1
            card.get_by_text("View both source passages").click()
            assert "gradient descent optimization" in card.inner_text().lower()
            page.goto(f"{console}/library")
            page.get_by_role("button", name="Add content").click()
            disclosure = page.locator("dialog[open] .processing-disclosure")
            assert "SELAR server" in disclosure.inner_text()
            assert "Google Gemini" in disclosure.inner_text()
            assert "Data export and account data removal are not available" in disclosure.inner_text()
            page.goto(f"{console}/reader?docId={latest}")
            card = page.locator(".mental-link-card").filter(has_text="concept overlap")
            card.get_by_role("button", name="Confirm").click()
            page.get_by_text("confirmed", exact=True).wait_for(timeout=10000)
            browser.close()
        confirmed = _request(client, "GET", f"{api}/api/mental-model-links?document_id={latest}", token)
        assert len(confirmed) == 1 and confirmed[0]["status"] == "confirmed"
        graph = _request(client, "GET", f"{api}/api/graph", token)
        assert any(edge["id"] == link["id"] and edge["state"] == "confirmed" for edge in graph["edges"])
        # A changed snapshot invalidates the witness, not merely its display.
        async def stale():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                await conn.execute("UPDATE documents SET content_hash='stale-fixture-replaced' WHERE id=$1", prior)
            finally:
                await conn.close()
        asyncio.run(stale())
        assert _request(client, "GET", f"{api}/api/mental-model-links?document_id={latest}", token) == []
        _request(client, "POST", f"{api}/api/mental-model-links/{link['id']}/respond", token,
                 expected=404, json={"action": "confirmed"})
        assert not any(edge["id"] == link["id"] for edge in _request(client, "GET", f"{api}/api/graph", token)["edges"])
