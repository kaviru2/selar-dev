"""Real Go API -> durable queue -> Python worker -> PostgreSQL -> Next/Chromium.

Only Gemini embeddings/generation are replaced. Requires a throwaway migrated
pgvector database, TEST_API_BINARY and TEST_CONSOLE_DIR; never point at user data.
"""
import asyncio
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
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
PDF_LINE = "Gradient descent optimization reduces a fabricated error score in a toy library."


def fabricated_pdf(line):
    """One text-only page, built by the already-required PyMuPDF dependency."""
    import fitz
    document = fitz.open()
    page = document.new_page()
    page.insert_text((72, 100), line, fontsize=11)
    data = document.tobytes()
    document.close()
    return data


def test_fabricated_pdf_fixture_extracts_exact_page_chunk(tmp_path):
    from ingestion import extract_pdf
    from main import merge_word_bboxes

    pdf = fabricated_pdf(PDF_LINE)
    path = tmp_path / "fabricated.pdf"
    path.write_bytes(pdf)
    source = extract_pdf(str(path), "synthetic-fixture", 500, merge_word_bboxes)
    assert source.content_hash == hashlib.sha256(pdf).hexdigest()
    assert source.page_count == 1
    assert len(source.chunks) == 1
    assert source.chunks[0]["text"] == PDF_LINE
    assert source.chunks[0]["locator"] == {"kind": "pdf", "page": 1, "block_index": 0}


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
    api_port, next_port, worker_port = _port(), _port(), _port()
    worker = subprocess.Popen([sys.executable, "-m", "uvicorn", "synthetic_chat_worker:app",
        "--host", "127.0.0.1", "--port", str(worker_port)],
        cwd=Path(__file__).parent, env={**os.environ, "DATABASE_URL": db,
                                      "INGESTION_QUEUE_ENABLED": "false"})
    api = None
    next_server = None
    try:
        _ready(f"http://127.0.0.1:{worker_port}/health", worker)
        api = subprocess.Popen([binary], env={**os.environ, "DATABASE_URL": db,
            "PORT": str(api_port), "WORKER_URL": f"http://127.0.0.1:{worker_port}",
            "JWT_SECRET": "ci-synthetic-jwt-not-production", "APP_ENV": "development"})
        _ready(f"http://127.0.0.1:{api_port}/healthz", api)
        next_server = subprocess.Popen(["pnpm", "start", "-p", str(next_port)], cwd=console,
            env={**os.environ, "API_INTERNAL_URL": f"http://127.0.0.1:{api_port}"})
        _ready(f"http://127.0.0.1:{next_port}/login", next_server)
        yield f"http://127.0.0.1:{api_port}", f"http://127.0.0.1:{next_port}"
    finally:
        for process in (next_server, api, worker):
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
        async def stored_witnesses():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                row = await conn.fetchrow(
                    "SELECT source_evidence, target_evidence FROM mental_model_links WHERE id=$1 AND user_id=$2",
                    link["id"], owner["user"]["id"])
                assert row is not None
                return [json.loads(row["source_evidence"]), json.loads(row["target_evidence"])]
            finally:
                await conn.close()
        witnesses = asyncio.run(stored_witnesses())
        for side, doc_id, evidence in zip(("source", "target"), (latest, prior), witnesses):
            assert evidence["asserting_source_id"] == doc_id
            assert evidence["quote"] == link[f"{side}_quote"] == link[f"{side}_evidence"]
            assert "gradient descent optimization" in evidence["quote"].lower()
            assert evidence["source_snapshot_hash"] and evidence["chunk_id"] == link[f"{side}_evidence_chunk_id"]
        assert _request(client, "GET", f"{api}/api/mental-model-links", foreign) == []
        _request(client, "GET", f"{api}/api/mental-model-links/{link['id']}/preview", foreign, expected=404)
        _request(client, "POST", f"{api}/api/mental-model-links/{link['id']}/respond", foreign,
                 expected=404, json={"action": "confirmed", "revision": link["revision"]})
        preview = _request(client, "GET", f"{api}/api/mental-model-links/{link['id']}/preview", token)
        assert preview["revision"] == link["revision"] == 0 and preview["history"] == []
        assert preview["source_quote"] and preview["target_quote"]
        reviewed_edge_id = f"reviewed:{link['id']}"
        assert not any(edge["id"] == reviewed_edge_id for edge in _request(client, "GET", f"{api}/api/graph", foreign)["edges"])
        # An authenticated account must not self-report unfinished quiz outcomes.
        async def owned_concept():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return await conn.fetchval("SELECT id FROM concepts WHERE user_id=$1 LIMIT 1",
                                           owner["user"]["id"])
            finally:
                await conn.close()
        concept_id = asyncio.run(owned_concept())
        assert concept_id is not None
        for signal in ("quiz_success", "quiz_failure"):
            _request(client, "POST", f"{api}/api/learner-signals", token, expected=409,
                json={"concept_id": str(concept_id), "signal": signal,
                      "idempotency_id": f"invented-{signal}"})
        async def unverified_outcomes():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return await conn.fetchval(
                    "SELECT count(*) FROM learning_events WHERE user_id=$1 "
                    "AND event_type IN ('quiz_success', 'quiz_failure')", owner["user"]["id"])
            finally:
                await conn.close()
        assert asyncio.run(unverified_outcomes()) == 0
        # A similarity-only near negative may be retrieved; it must not become a
        # grounded candidate without a two-sided exact assertion.
        neg = _request(client, "POST", f"{api}/api/documents/add", token, expected=202,
            json={"source_type": "text", "title": "Fabricated notebook C", "text": NEGATIVE})["document"]["id"]
        asyncio.run(_drain_job(neg))
        assert _request(client, "GET", f"{api}/api/mental-model-links?document_id={neg}", token) == []

        # Actual multipart route and queued PDF extraction, not a pasted-text surrogate.
        pdf = fabricated_pdf(PDF_LINE)
        uploaded = _request(client, "POST", f"{api}/api/documents/upload", token, expected=202,
            files={"file": ("fabricated-library.pdf", pdf, "application/pdf")})
        pdf_id = uploaded["document"]["id"]
        assert uploaded["document"]["status"] == "processing"
        asyncio.run(_drain_job(pdf_id))
        content = _request(client, "GET", f"{api}/api/documents/{pdf_id}/content", token)
        assert content["document"]["content_hash"] == hashlib.sha256(pdf).hexdigest()
        assert content["document"]["page_count"] == 1
        assert len(content["blocks"]) == 1
        block = content["blocks"][0]
        assert block["text"] == PDF_LINE
        assert block["locator"]["kind"] == "pdf" and block["locator"]["page"] == 1
        assert block["locator"]["block_index"] == 0 and block["locator"]["bboxes"]
        async def pdf_chunk():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return [dict(row) for row in await conn.fetch(
                    "SELECT id, content, page_start, page_end, locator FROM chunks WHERE document_id=$1 AND user_id=$2",
                    pdf_id, owner["user"]["id"])]
            finally:
                await conn.close()
        chunks = asyncio.run(pdf_chunk())
        assert len(chunks) == 1 and chunks[0]["content"] == PDF_LINE
        assert chunks[0]["page_start"] == chunks[0]["page_end"] == 1
        assert json.loads(chunks[0]["locator"])["kind"] == "pdf"
        assert json.loads(chunks[0]["locator"])["block_index"] == 0
        raw_pdf = client.get(f"{api}/api/documents/{pdf_id}/pdf", headers={"Authorization": f"Bearer {token}"})
        assert raw_pdf.status_code == 200 and raw_pdf.content == pdf
        for path in ("content", "pdf", "mental-model"):
            response = client.get(f"{api}/api/documents/{pdf_id}/{path}",
                headers={"Authorization": f"Bearer {foreign}"})
            assert response.status_code == 404, (path, response.status_code)
        pdf_links = _request(client, "GET", f"{api}/api/mental-model-links?document_id={pdf_id}", token)
        assert pdf_links, "PDF extraction must produce a live grounded candidate"
        pdf_link = next(item for item in pdf_links if item["source_document_id"] == pdf_id)
        assert pdf_link["source_evidence_chunk_id"] == str(chunks[0]["id"])
        assert pdf_link["source_quote"] == PDF_LINE
        assert pdf_link["source_locator"]["kind"] == "pdf" and pdf_link["source_locator"]["page"] == 1
        async def pdf_witness():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return json.loads(await conn.fetchval(
                    "SELECT source_evidence FROM mental_model_links WHERE id=$1 AND user_id=$2",
                    pdf_link["id"], owner["user"]["id"]))
            finally:
                await conn.close()
        witness = asyncio.run(pdf_witness())
        assert witness["asserting_source_id"] == pdf_id
        assert witness["source_snapshot_hash"] == hashlib.sha256(pdf).hexdigest()
        assert witness["chunk_id"] == str(chunks[0]["id"]) and witness["quote"] == PDF_LINE
        assert witness["locator"]["kind"] == "pdf" and witness["locator"]["page"] == 1
        _request(client, "GET", f"{api}/api/mental-model-links/{pdf_link['id']}/preview", foreign, expected=404)

        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(headless=True)
            context = browser.new_context()
            page = context.new_page()
            page.goto(f"{console}/login")
            login = context.request.post(f"{console}/api/auth/login", data={
                "email": f"owner-{suffix}@example.invalid", "password": PASSWORD})
            assert login.status == 200, login.text()
            quiz = page.goto(f"{console}/quiz")
            assert quiz is not None and quiz.status == 404
            page.goto(f"{console}/reader?docId={pdf_id}&page=1")
            page.locator(".pdf-page-container .textLayer").get_by_text(PDF_LINE).wait_for(timeout=20000)
            assert "1 / 1" in page.locator(".page-indicator").first.inner_text()
            page.goto(f"{console}/reader?docId={latest}")
            card = page.locator(".mental-link-card").filter(has_text="Fabricated notebook A")
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
            card = page.locator(".mental-link-card").filter(has_text="Fabricated notebook A")
            preview_button = page.get_by_role("button", name=(
                f"Preview grounded assertion from {link['source_document_title']} "
                f"to {link['target_document_title']}"))
            preview_button.focus()
            preview_button.press("Enter")
            review = page.get_by_role("region", name="Grounded assertion review")
            review.get_by_text("not a retrieval success").wait_for(timeout=10000)
            heading = review.get_by_role("heading", name="Review concept overlap · Revision 0")
            assert heading.evaluate("element => element === document.activeElement"), "keyboard-opened review must receive focus"
            page.keyboard.press("Tab")
            assert review.get_by_role("link", name="Open source in reader").evaluate(
                "element => element === document.activeElement"), "source witness must be next in keyboard order"
            assert "gradient descent optimization" in review.inner_text().lower()
            reflection = page.get_by_role("region", name="Optional evidence reflection")
            reflection.get_by_role("textbox", name="Your explanation of how the claims connect").fill(
                "The two toy examples both describe gradient descent optimization.")
            reflection.get_by_role("button", name="Try recalling the relationship").click()
            assert reflection.locator("q").count() == 0
            reflection.get_by_role("textbox", name="Your optional recall of the relationship").fill(
                "Both fabricated examples reduce a toy error score.")
            reflection.get_by_role("button", name="Revisit source quotes").click()
            assert reflection.locator("q").count() == 2
            reflection.get_by_role("button", name="Discard drafts").click()
            assert reflection.get_by_role("textbox", name="Your explanation of how the claims connect").input_value() == ""
            assert reflection.get_by_role("textbox", name="Your optional recall of the relationship").input_value() == ""
            assert "No answer was saved" in reflection.get_by_role("status").inner_text()
            with page.expect_response(lambda response: (
                response.request.method == "POST"
                and f"/api/mental-model-links/{link['id']}/respond" in response.url
            )) as submitted_review:
                review.get_by_role("button", name="confirmed").click()
            assert submitted_review.value.status == 200
            page.get_by_text("confirmed", exact=True).first.wait_for(timeout=10000)
            browser.close()
        confirmed = _request(client, "GET", f"{api}/api/mental-model-links?document_id={latest}", token)
        assert any(item["id"] == link["id"] and item["status"] == "confirmed" for item in confirmed)
        review = _request(client, "GET", f"{api}/api/mental-model-links/{link['id']}/preview", token)
        assert review["revision"] == 1 and len(review["history"]) == 1
        assert review["history"][0]["action"] == "confirmed"
        _request(client, "POST", f"{api}/api/mental-model-links/{link['id']}/respond", token,
                 expected=409, json={"action": "retracted", "revision": 0,
                                     "reason": "stale synthetic review"})
        graph = _request(client, "GET", f"{api}/api/graph", token)
        assert any(edge["id"] == reviewed_edge_id and edge["mental_link_id"] == link["id"]
                   and edge["state"] == "confirmed" for edge in graph["edges"])
        # Human decisions are revision-bound annotations, never retrieval success.
        def decide(candidate, action, revision, **fields):
            return _request(client, "POST", f"{api}/api/mental-model-links/{candidate['id']}/respond",
                token, json={"action": action, "revision": revision, **fields})
        def assert_history(candidate, expected):
            current = _request(client, "GET", f"{api}/api/mental-model-links/{candidate['id']}/preview", token)
            assert current["revision"] == len(expected)
            assert [(e["action"], e["before_status"], e["after_status"], e["reason"],
                     e.get("target_revision")) for e in current["history"]] == expected
            return current
        correction = "Compare fabricated optimization settings"
        decide(link, "relabeled", 1, label=correction, reason="human correction note")
        corrected = assert_history(link, [
            ("confirmed", "candidate", "confirmed", "", None),
            ("relabeled", "confirmed", "relabeled", "human correction note", None)])
        assert corrected["user_label"] == correction and corrected["link_type"] == "concept_overlap"
        edge = next(e for e in _request(client, "GET", f"{api}/api/graph", token)["edges"]
                    if e["id"] == reviewed_edge_id)
        assert edge["state"] == "relabeled" and edge["relation"] == "concept_overlap"
        assert edge["review_revision"] == 2
        decide(link, "retracted", 2, reason="withdraw toy interpretation")
        retracted = assert_history(link, [
            ("confirmed", "candidate", "confirmed", "", None),
            ("relabeled", "confirmed", "relabeled", "human correction note", None),
            ("retracted", "relabeled", "archived", "withdraw toy interpretation", None)])
        assert retracted["status"] == "archived"
        assert not any(e["id"] == reviewed_edge_id for e in _request(client, "GET", f"{api}/api/graph", token)["edges"])
        decide(link, "rolled_back", 3, target_revision=2, reason="restore corrected toy interpretation")
        restored = assert_history(link, [
            ("confirmed", "candidate", "confirmed", "", None),
            ("relabeled", "confirmed", "relabeled", "human correction note", None),
            ("retracted", "relabeled", "archived", "withdraw toy interpretation", None),
            ("rolled_back", "archived", "relabeled", "restore corrected toy interpretation", 2)])
        assert restored["status"] == "relabeled" and restored["user_label"] == correction
        edge = next(e for e in _request(client, "GET", f"{api}/api/graph", token)["edges"]
                    if e["id"] == reviewed_edge_id)
        assert edge["state"] == "relabeled" and edge["review_revision"] == 4
        decide(pdf_link, "rejected", 0, reason="fabricated comparison not endorsed")
        rejected = assert_history(pdf_link, [
            ("rejected", "candidate", "rejected", "fabricated comparison not endorsed", None)])
        assert rejected["status"] == "rejected"
        assert not any(e["id"] == f"reviewed:{pdf_link['id']}" for e in
                       _request(client, "GET", f"{api}/api/graph", token)["edges"])
        _request(client, "POST", f"{api}/api/mental-model-links/{pdf_link['id']}/respond",
            foreign, expected=404, json={"action": "rolled_back", "revision": 1,
                                         "target_revision": 0, "reason": "foreign attempt"})
        async def retrieval_successes():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return await conn.fetchval(
                    "SELECT count(*) FROM learner_concept_state WHERE user_id=$1 AND success_count>0",
                    owner["user"]["id"])
            finally:
                await conn.close()
        assert asyncio.run(retrieval_successes()) == 0
        # A changed snapshot invalidates the witness, not merely its display.
        async def stale():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                await conn.execute("UPDATE documents SET content_hash='stale-fixture-replaced' WHERE id=$1", prior)
            finally:
                await conn.close()
        asyncio.run(stale())
        assert not any(item["id"] == link["id"] for item in
                       _request(client, "GET", f"{api}/api/mental-model-links?document_id={latest}", token))
        _request(client, "GET", f"{api}/api/mental-model-links/{link['id']}/preview", token, expected=404)
        _request(client, "POST", f"{api}/api/mental-model-links/{link['id']}/respond", token,
                 expected=404, json={"action": "retracted", "revision": 4,
                                     "reason": "source snapshot changed"})
        assert not any(edge["id"] == reviewed_edge_id for edge in _request(client, "GET", f"{api}/api/graph", token)["edges"])
