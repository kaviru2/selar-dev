"""Fabricated WEB path; no live HTTP destination or model call is permitted."""
import asyncio
import hashlib
import json
import os
import uuid
from io import BytesIO

import asyncpg

from test_synthetic_service_e2e import PASSWORD, _drain_job, _request, services

import httpx
import pytest
from PIL import Image

# TEST-NET ranges are rejected by the production public-IP guard. This numeric
# address passes that guard, but the test must intercept every outbound HTTP call.
ARTICLE_URL = "http://8.8.8.8/fictional-garden"
ARTICLE_TEXT = ("Gradient descent optimization reduces a fabricated error score in a toy orchard. "
                "The invented gardener measures twenty paper trees and repeats the same tiny "
                "calculation for each imaginary season. No observations about real people or "
                "real experiments are represented by this fictional article.")
HTML = ("<html><head><title>Fabricated orchard article</title></head><body>"
        "<article><h1>Fabricated orchard article</h1><p>" + ARTICLE_TEXT +
        "</p><figure><img src='/orchard.png' alt='Toy orchard diagram'>"
        "<figcaption>Invented orchard diagram</figcaption></figure></article></body></html>")


def install_fabricated_transport(monkeypatch):
    """Only interception is mocked: production URL validation/fetch stays intact.

    The pinned transport still rewrites Host/SNI and the target IP; interception
    happens only beneath it, at the HTTP network boundary. MockTransport opens
    no socket and supplies no peer, so this does NOT prove a real peer binding,
    DNS rebinding resistance, proxy behavior, or live web access.
    """
    import ingestion

    image = Image.new("RGB", (200, 140), color=(80, 140, 80))
    output = BytesIO()
    image.save(output, format="PNG")
    requests = []
    paths = {
        "/robots.txt": (b"User-agent: *\nAllow: /fictional-garden\n", "text/plain"),
        "/fictional-garden": (HTML.encode(), "text/html"),
        "/orchard.png": (output.getvalue(), "image/png"),
    }

    def respond(request):
        assert request.url.host == "8.8.8.8" and request.url.scheme == "http", request.url
        assert request.headers["Host"] == "8.8.8.8"
        assert request.extensions["sni_hostname"] == "8.8.8.8"
        assert request.method == "GET"
        requests.append(request.url.path)
        assert request.url.path in paths, f"unexpected fixture request: {request.url}"
        data, content_type = paths[request.url.path]
        return httpx.Response(200, content=data, headers={"content-type": content_type})

    transport = httpx.MockTransport(respond)

    async def intercepted_socket(self, request):
        assert isinstance(self, ingestion._PinnedPublicTransport)
        assert self.address == "8.8.8.8"
        return await transport.handle_async_request(request)

    monkeypatch.setattr(httpx.AsyncHTTPTransport, "handle_async_request", intercepted_socket)
    return requests, output.getvalue()


def test_fabricated_web_extracts_through_public_url_checks(monkeypatch):
    import ingestion

    requests, image = install_fabricated_transport(monkeypatch)
    assert asyncio.run(ingestion.validate_public_url(ARTICLE_URL)) == ARTICLE_URL
    with pytest.raises(ingestion.UnsafeSourceURL):
        asyncio.run(ingestion.validate_public_url("http://127.0.0.1/fictional-garden"))
    result = asyncio.run(ingestion.extract_web(ARTICLE_URL, "invented-web", 500))
    assert requests == ["/robots.txt", "/fictional-garden", "/orchard.png"]
    assert result.canonical_url == ARTICLE_URL
    assert result.title == "Fabricated orchard article"
    assert result.mime_type == "text/html"
    assert any(ARTICLE_TEXT in chunk["text"] for chunk in result.chunks)
    assert len(result.assets) == 1
    assert result.assets[0]["source_url"] == "http://8.8.8.8/orchard.png"
    assert result.assets[0]["width"] == 200 and result.assets[0]["height"] == 140
    assert result.assets[0]["storage_path"] and image


@pytest.mark.timeout(180)
def test_web_api_queue_snapshot_owner_reader_without_promotion(services, monkeypatch):
    """Actual Go/queue/DB/Chromium; only HTTP fixture and Gemini are intercepted."""
    import main
    from playwright.sync_api import sync_playwright

    requests, image = install_fabricated_transport(monkeypatch)
    monkeypatch.setattr(main, "DATABASE_URL", os.environ["TEST_DATABASE_URL"])
    monkeypatch.setattr(main, "embed_text_documents", lambda texts, title="": [
        [1.0] + [0.0] * 3071 for _ in texts])

    class Models:
        def generate_content(self, **_):
            return type("Result", (), {"text": json.dumps({
                "main_claim": "The invented article describes gradient descent optimization.",
                "key_concepts": [{"name": "gradient descent optimization",
                                  "description": "A toy calculation", "evidence_chunk_index": 0}],
                "assumptions": [], "open_questions": [], "domain": "fabricated example",
                "concept_edges": []})})()

    monkeypatch.setattr(main, "client", type("Client", (), {"models": Models()})())
    api, console = services
    suffix = uuid.uuid4().hex
    with httpx.Client() as client:
        accounts = [
            _request(client, "POST", f"{api}/auth/register", expected=201,
                     json={"email": f"web-{label}-{suffix}@example.invalid", "password": PASSWORD})
            for label in ("owner", "foreign")]
        owner, foreign = accounts[0]["token"], accounts[1]["token"]
        owner_id = accounts[0]["user"]["id"]
        prior = _request(client, "POST", f"{api}/api/documents/add", owner, expected=202,
                         json={"source_type": "text", "title": "Invented comparison notebook",
                               "text": "# Invented comparison notebook\nGradient descent optimization reduces a fabricated error score in a toy orchard.\n"})["document"]["id"]
        asyncio.run(_drain_job(prior))
        web = _request(client, "POST", f"{api}/api/documents/add", owner, expected=202,
                       json={"source_type": "web", "source_url": ARTICLE_URL,
                             "title": "Untrusted submitted title"})["document"]["id"]
        asyncio.run(_drain_job(web))
        assert requests == ["/robots.txt", "/fictional-garden", "/orchard.png"]
        content = _request(client, "GET", f"{api}/api/documents/{web}/content", owner)
        doc = content["document"]
        assert doc["status"] == "ready" and doc["source_type"] == "web"
        assert doc["source_url"] == ARTICLE_URL and doc["title"] == "Fabricated orchard article"
        assert len(content["assets"]) == 1
        asset = content["assets"][0]
        assert asset["source_url"] == "http://8.8.8.8/orchard.png"
        assert asset["caption"] == "Invented orchard diagram"
        assert any(block["text"] == ARTICLE_TEXT for block in content["blocks"])
        figure = next(block for block in content["blocks"] if block["kind"] == "figure")
        assert figure["locator"] == {"kind": "web", "block_index": figure["block_index"],
                                      "source_url": asset["source_url"]}
        raw_asset = client.get(f"{api}/api/documents/{web}/assets/{asset['id']}",
                               headers={"Authorization": f"Bearer {owner}"})
        assert raw_asset.status_code == 200 and raw_asset.content == image
        for path in ("content", "mental-model", f"assets/{asset['id']}"):
            denied = client.get(f"{api}/api/documents/{web}/{path}",
                                headers={"Authorization": f"Bearer {foreign}"})
            assert denied.status_code == 404, (path, denied.status_code)

        async def persisted():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                chunks = await conn.fetch("SELECT id, content, locator FROM chunks "
                                          "WHERE document_id=$1 AND user_id=$2", web, owner_id)
                return [dict(row) for row in chunks]
            finally:
                await conn.close()
        chunks = asyncio.run(persisted())
        assert len(chunks) == 1 and chunks[0]["content"] == ARTICLE_TEXT
        locator = json.loads(chunks[0]["locator"])
        assert locator == {"kind": "block", "block_index": 1,
                           "heading": "Fabricated orchard article", "word_start": 0,
                           "word_end": len(ARTICLE_TEXT.split())}
        links = _request(client, "GET", f"{api}/api/mental-model-links?document_id={web}", owner)
        link = next(item for item in links if item["source_document_id"] == web)
        assert link["status"] == "candidate" and link["link_type"] == "concept_overlap"
        assert link["target_document_id"] == prior
        assert link["source_evidence_chunk_id"] == str(chunks[0]["id"])
        assert link["source_quote"] == ARTICLE_TEXT and link["source_locator"] == locator
        async def stored_witness():
            conn = await asyncpg.connect(os.environ["TEST_DATABASE_URL"])
            try:
                return json.loads(await conn.fetchval(
                    "SELECT source_evidence FROM mental_model_links WHERE id=$1 AND user_id=$2",
                    link["id"], owner_id))
            finally:
                await conn.close()
        witness = asyncio.run(stored_witness())
        assert witness["asserting_source_id"] == web
        assert witness["chunk_id"] == str(chunks[0]["id"])
        assert witness["quote"] == ARTICLE_TEXT and witness["locator"] == locator
        assert witness["source_snapshot_hash"] == doc["content_hash"]
        assert len(doc["content_hash"]) == len(hashlib.sha256(b"").hexdigest())
        assert _request(client, "GET", f"{api}/api/mental-model-links", foreign) == []
        _request(client, "GET", f"{api}/api/mental-model-links/{link['id']}/preview",
                 foreign, expected=404)
        assert not any(edge.get("mental_link_id") == link["id"] for edge in
                       _request(client, "GET", f"{api}/api/graph", owner)["edges"])
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(headless=True)
            context = browser.new_context()
            page = context.new_page()
            page.goto(f"{console}/login")
            login = context.request.post(f"{console}/api/auth/login", data={
                "email": f"web-owner-{suffix}@example.invalid", "password": PASSWORD})
            assert login.status == 200, login.text()
            page.goto(f"{console}/reader?docId={web}")
            reader = page.locator(".article-reader")
            reader.get_by_text(ARTICLE_TEXT).wait_for(timeout=20000)
            assert reader.locator(".article-byline a").get_attribute("href") == ARTICLE_URL
            assert reader.locator("figure figcaption").inner_text() == "Invented orchard diagram"
            assert reader.locator("figure img").count() == 1
            assert "Invented comparison notebook" in page.locator(".mental-link-card").inner_text()
            browser.close()
        assert requests == ["/robots.txt", "/fictional-garden", "/orchard.png"]
