"""Source adapters for research-auditable, multimodal SELAR ingestion."""

from __future__ import annotations

import asyncio
import hashlib
import ipaddress
import os
import re
import socket
from dataclasses import dataclass, field
from importlib.metadata import PackageNotFoundError, version
from io import BytesIO
from pathlib import Path
from typing import Any
from urllib.parse import parse_qsl, urlencode, urljoin, urlparse, urlunparse
from urllib.robotparser import RobotFileParser

import fitz
import httpx
import pdfplumber
import trafilatura
from bs4 import BeautifulSoup
from PIL import Image

MAX_SOURCE_BYTES = int(os.getenv("MAX_SOURCE_BYTES", str(10 << 20)))
MAX_IMAGE_BYTES = int(os.getenv("MAX_IMAGE_BYTES", str(5 << 20)))
MAX_REDIRECTS = 5
MAX_ARTICLE_IMAGES = int(os.getenv("MAX_ARTICLE_IMAGES", "12"))
MAX_PDF_PAGE_RENDERS = int(os.getenv("MAX_PDF_PAGE_RENDERS", "12"))
USER_AGENT = "SELAR-Research-Ingestion/1.0 (+https://github.com/kaviru2/selar-dev)"


@dataclass
class NormalizedSource:
    title: str
    authors: str = ""
    year: int = 0
    page_count: int = 0
    canonical_url: str = ""
    content_hash: str = ""
    mime_type: str = ""
    metadata: dict[str, Any] = field(default_factory=dict)
    blocks: list[dict[str, Any]] = field(default_factory=list)
    chunks: list[dict[str, Any]] = field(default_factory=list)
    assets: list[dict[str, Any]] = field(default_factory=list)
    extractor: str = ""
    extractor_version: str = ""


class UnsafeSourceURL(ValueError):
    pass


def package_version(name: str) -> str:
    try:
        return version(name)
    except PackageNotFoundError:
        return "unknown"


def clean_text(text: str) -> str:
    text = re.sub(r"(?<=\w)-\s+(?=\w)", "", text)
    return re.sub(r"[ \t]+", " ", text).strip()


def canonicalize_url(raw: str) -> str:
    parsed = urlparse(raw.strip())
    if parsed.scheme.lower() not in {"http", "https"} or not parsed.hostname:
        raise UnsafeSourceURL("only public http and https URLs are supported")
    host = parsed.hostname.lower().rstrip(".")
    if ":" in host:
        host = f"[{host}]"
    port = parsed.port
    if port and not ((parsed.scheme == "http" and port == 80) or (parsed.scheme == "https" and port == 443)):
        host = f"{host}:{port}"
    query = urlencode([
        (key, value) for key, value in parse_qsl(parsed.query, keep_blank_values=True)
        if not key.lower().startswith("utm_") and key.lower() not in {"fbclid", "gclid"}
    ])
    return urlunparse((parsed.scheme.lower(), host, parsed.path or "/", "", query, ""))


def _is_public_ip(value: str) -> bool:
    ip = ipaddress.ip_address(value)
    return not (
        ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_multicast
        or ip.is_reserved or ip.is_unspecified
    )


async def _resolve_public_url(raw: str) -> tuple[str, str]:
    canonical = canonicalize_url(raw)
    parsed = urlparse(canonical)
    try:
        default_port = 80 if parsed.scheme == "http" else 443
        records = await asyncio.to_thread(socket.getaddrinfo, parsed.hostname, parsed.port or default_port, type=socket.SOCK_STREAM)
    except socket.gaierror as exc:
        raise UnsafeSourceURL("source host could not be resolved") from exc
    addresses = {str(record[4][0]) for record in records}
    if not addresses or any(not _is_public_ip(address) for address in addresses):
        raise UnsafeSourceURL("source resolves to a blocked network address")
    # All records must be public; connect only to one of these validated IPs.
    return canonical, sorted(addresses)[0]


async def validate_public_url(raw: str) -> str:
    canonical, _ = await _resolve_public_url(raw)
    return canonical


class _PinnedPublicTransport(httpx.AsyncHTTPTransport):
    """Connect to the validated IP without changing HTTP authority or TLS identity."""

    def __init__(self, address: str):
        super().__init__(trust_env=False)
        self.address = address

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        original_url = request.url
        request.headers["Host"] = original_url.netloc.decode("ascii")
        request.extensions["sni_hostname"] = original_url.raw_host.decode("ascii")
        request.url = original_url.copy_with(host=self.address)
        return await super().handle_async_request(request)


async def fetch_public_url(
    raw: str,
    *,
    max_bytes: int = MAX_SOURCE_BYTES,
    allowed_content_types: tuple[str, ...] = ("text/html", "text/plain", "application/xhtml+xml"),
) -> tuple[bytes, str, str]:
    current, address = await _resolve_public_url(raw)
    timeout = httpx.Timeout(20.0, connect=8.0)
    for _ in range(MAX_REDIRECTS + 1):
        async with httpx.AsyncClient(
            timeout=timeout, follow_redirects=False, trust_env=False,
            transport=_PinnedPublicTransport(address), headers={"User-Agent": USER_AGENT},
        ) as client:
            async with client.stream("GET", current) as response:
                if response.status_code in {301, 302, 303, 307, 308}:
                    location = response.headers.get("location")
                    if not location:
                        raise ValueError("redirect response did not include a location")
                    current, address = await _resolve_public_url(urljoin(current, location))
                    continue
                response.raise_for_status()
                network_stream = response.extensions.get("network_stream")
                peer = network_stream.get_extra_info("server_addr") if network_stream else None
                peer_address = peer[0] if isinstance(peer, tuple) and peer else peer
                if peer_address and not _is_public_ip(str(peer_address)):
                    raise UnsafeSourceURL("source connected to a blocked network address")
                content_type = response.headers.get("content-type", "").split(";", 1)[0].lower()
                if not any(content_type.startswith(prefix) for prefix in allowed_content_types):
                    raise ValueError(f"unsupported source content type: {content_type or 'unknown'}")
                declared = response.headers.get("content-length")
                if declared and int(declared) > max_bytes:
                    raise ValueError("source exceeds the configured size limit")
                data = bytearray()
                async for chunk in response.aiter_bytes():
                    data.extend(chunk)
                    if len(data) > max_bytes:
                        raise ValueError("source exceeds the configured size limit")
                return bytes(data), current, content_type
    raise ValueError("source redirected too many times")


async def robots_allows(url: str) -> bool:
    parsed = urlparse(url)
    robots_url = urlunparse((parsed.scheme, parsed.netloc, "/robots.txt", "", "", ""))
    try:
        data, _, _ = await fetch_public_url(
            robots_url,
            max_bytes=512 * 1024,
            allowed_content_types=("text/plain", "text/"),
        )
    except httpx.HTTPStatusError as exc:
        return exc.response.status_code in {404, 410}
    except Exception:
        return True
    parser = RobotFileParser()
    parser.set_url(robots_url)
    parser.parse(data.decode("utf-8", errors="replace").splitlines())
    return parser.can_fetch(USER_AGENT, url)


def markdown_blocks(markdown: str) -> list[dict[str, Any]]:
    blocks: list[dict[str, Any]] = []
    paragraph: list[str] = []

    def flush() -> None:
        if paragraph:
            text = clean_text(" ".join(paragraph))
            if text:
                blocks.append({"kind": "paragraph", "text": text})
            paragraph.clear()

    for raw_line in markdown.splitlines():
        line = raw_line.strip()
        if not line:
            flush()
            continue
        heading = re.match(r"^(#{1,6})\s+(.+)$", line)
        if heading:
            flush()
            blocks.append({"kind": "heading", "text": clean_text(heading.group(2)), "level": len(heading.group(1))})
        elif line.startswith(("- ", "* ", "+ ")) or re.match(r"^\d+[.)]\s+", line):
            flush()
            blocks.append({"kind": "list", "text": clean_text(re.sub(r"^(?:[-*+]\s+|\d+[.)]\s+)", "", line))})
        elif line.startswith(">"):
            flush()
            blocks.append({"kind": "quote", "text": clean_text(line.lstrip("> "))})
        else:
            paragraph.append(line)
    flush()
    for index, block in enumerate(blocks):
        block["block_index"] = index
        block["locator"] = {"kind": "block", "block_index": index}
        block.setdefault("metadata", {})
    return blocks


def chunks_from_blocks(blocks: list[dict[str, Any]], word_limit: int) -> list[dict[str, Any]]:
    chunks: list[dict[str, Any]] = []
    heading = ""
    for block in blocks:
        if block["kind"] == "heading":
            heading = block["text"]
            continue
        words = block.get("text", "").split()
        for offset in range(0, len(words), word_limit):
            text = clean_text(" ".join(words[offset:offset + word_limit]))
            if not text:
                continue
            locator = dict(block.get("locator", {}))
            locator.update({"heading": heading, "word_start": offset, "word_end": offset + len(text.split())})
            chunks.append({"text": text, "page": int(locator.get("page", 1)), "bboxes": [], "locator": locator, "modality": "text"})
    return chunks


def _save_image(data: bytes, asset_dir: Path, suggested_ext: str = "png") -> tuple[str, str, int, int, str] | None:
    try:
        image = Image.open(BytesIO(data))
        image.load()
        width, height = image.size
        if width < 160 or height < 120:
            return None
        image_format = (image.format or suggested_ext).lower()
        if image_format == "jpg":
            image_format = "jpeg"
        if image_format not in {"png", "jpeg", "webp", "gif"}:
            image_format = "png"
        digest = hashlib.sha256(data).hexdigest()
        asset_dir.mkdir(parents=True, exist_ok=True)
        path = asset_dir / f"{digest}.{image_format}"
        if not path.exists():
            path.write_bytes(data)
        return str(path), f"image/{image_format}", width, height, digest
    except Exception:
        return None


def is_likely_decorative_image(source_url: str, alt: str, caption: str) -> bool:
    """Reject common page chrome while retaining captioned explanatory visuals."""
    if caption:
        return False
    description = f"{source_url} {alt}".lower()
    return any(marker in description for marker in (
        "avatar", "profile picture", "profile photo", "author photo",
        "/avatars/", "logo", "site icon", "favicon", "emoji", "badge",
    ))


def is_weak_image_alt(alt: str) -> bool:
    return bool(re.fullmatch(r"image/(?:png|jpe?g|webp|gif)", alt.strip().lower()))


async def extract_web(url: str, doc_id: str, word_limit: int) -> NormalizedSource:
    if not await robots_allows(url):
        raise ValueError("the source robots policy does not allow SELAR ingestion")
    data, final_url, mime_type = await fetch_public_url(url)
    html = data.decode("utf-8", errors="replace")
    markdown = trafilatura.extract(
        html,
        url=final_url,
        output_format="markdown",
        include_links=True,
        include_tables=True,
        favor_precision=True,
    ) or ""
    if len(markdown.split()) < 30:
        raise ValueError("the page did not contain enough extractable article text")
    metadata = trafilatura.extract_metadata(html, default_url=final_url)
    title = clean_text(getattr(metadata, "title", "") or "") or urlparse(final_url).hostname or "Web article"
    author = clean_text(getattr(metadata, "author", "") or "")
    date = str(getattr(metadata, "date", "") or "")
    year_match = re.match(r"(19|20)\d{2}", date)
    blocks = markdown_blocks(markdown)
    asset_dir = Path("/tmp/selar_uploads") / doc_id / "assets"
    assets: list[dict[str, Any]] = []
    soup = BeautifulSoup(html, "html.parser")
    seen_urls: set[str] = set()
    seen_hashes: set[str] = set()
    for image_node in soup.select("article img, main img, figure img"):
        if len(assets) >= MAX_ARTICLE_IMAGES:
            break
        source_url = image_node.get("src") or image_node.get("data-src")
        if not source_url:
            continue
        source_url = urljoin(final_url, source_url)
        if source_url in seen_urls:
            continue
        seen_urls.add(source_url)
        alt = clean_text(image_node.get("alt", ""))
        figure = image_node.find_parent("figure")
        caption_node = figure.find("figcaption") if figure else None
        caption = clean_text(caption_node.get_text(" ", strip=True) if caption_node else "")
        context_node = image_node.find_previous(["h2", "h3", "h4", "p"])
        description = clean_text(context_node.get_text(" ", strip=True) if context_node else "")[:500]
        if not description:
            description = f"Visual evidence from {title}"
        if not alt and not caption:
            continue
        if is_likely_decorative_image(source_url, alt, caption):
            continue
        try:
            image_bytes, resolved_url, image_mime = await fetch_public_url(
                source_url,
                max_bytes=MAX_IMAGE_BYTES,
                allowed_content_types=("image/",),
            )
        except Exception:
            continue
        saved = _save_image(image_bytes, asset_dir, image_mime.split("/")[-1])
        if not saved:
            continue
        storage_path, stored_mime, width, height, digest = saved
        if digest in seen_hashes:
            continue
        seen_hashes.add(digest)
        block_index = len(blocks)
        locator = {"kind": "web", "block_index": block_index, "source_url": resolved_url}
        display_label = caption or ("" if is_weak_image_alt(alt) else alt) or description
        blocks.append({
            "block_index": block_index, "kind": "figure", "text": display_label,
            "locator": locator, "metadata": {"alt_text": alt, "source_url": resolved_url},
        })
        assets.append({
            "block_index": block_index, "kind": "figure", "storage_path": storage_path,
            "source_url": resolved_url, "mime_type": stored_mime, "width": width, "height": height,
            "content_hash": digest, "caption": caption, "alt_text": alt, "description": description,
            "locator": locator,
        })
    normalized = NormalizedSource(
        title=title,
        authors=author,
        year=int(year_match.group(0)) if year_match else 0,
        canonical_url=canonicalize_url(final_url),
        content_hash=hashlib.sha256(markdown.encode()).hexdigest(),
        mime_type=mime_type,
        metadata={"published_at": date, "site_name": getattr(metadata, "sitename", "") or ""},
        blocks=blocks,
        assets=assets,
        extractor="trafilatura",
        extractor_version=package_version("trafilatura"),
    )
    normalized.chunks = chunks_from_blocks(blocks, word_limit)
    return normalized


def extract_text(raw_text: str, title: str, word_limit: int) -> NormalizedSource:
    blocks = markdown_blocks(raw_text)
    if not blocks:
        raise ValueError("text source is empty")
    normalized = NormalizedSource(
        title=title or "Pasted notes",
        content_hash=hashlib.sha256(raw_text.encode()).hexdigest(),
        mime_type="text/markdown",
        blocks=blocks,
        extractor="selar-markdown",
        extractor_version="1",
    )
    normalized.chunks = chunks_from_blocks(blocks, word_limit)
    return normalized


def extract_pdf(file_path: str, doc_id: str, word_limit: int, merge_bboxes, title: str = "") -> NormalizedSource:
    chunks: list[dict[str, Any]] = []
    blocks: list[dict[str, Any]] = []
    with pdfplumber.open(file_path) as pdf:
        page_count = len(pdf.pages)
        for page_num, page in enumerate(pdf.pages, start=1):
            words = page.extract_words(use_text_flow=True, x_tolerance=1, y_tolerance=3)
            if not words:
                continue
            for offset in range(0, len(words), word_limit):
                group = words[offset:offset + word_limit]
                text = clean_text(" ".join(word["text"] for word in group))
                if not text:
                    continue
                block_index = len(blocks)
                locator = {"kind": "pdf", "page": page_num, "block_index": block_index}
                bboxes = merge_bboxes(group, page.width, page.height)
                blocks.append({
                    "block_index": block_index, "kind": "paragraph", "text": text,
                    "locator": {**locator, "bboxes": bboxes}, "metadata": {},
                })
                chunks.append({"text": text, "page": page_num, "bboxes": bboxes, "locator": locator, "modality": "text"})

    assets: list[dict[str, Any]] = []
    asset_dir = Path("/tmp/selar_uploads") / doc_id / "assets"
    pdf_doc = fitz.open(file_path)
    seen_xrefs: set[int] = set()
    rendered_pages = 0
    for page_index in range(len(pdf_doc)):
        page = pdf_doc[page_index]
        for image_info in page.get_images(full=True):
            xref = image_info[0]
            if xref in seen_xrefs:
                continue
            seen_xrefs.add(xref)
            try:
                extracted = pdf_doc.extract_image(xref)
                saved = _save_image(extracted["image"], asset_dir, extracted.get("ext", "png"))
            except Exception:
                saved = None
            if not saved:
                continue
            storage_path, stored_mime, width, height, digest = saved
            locator = {"kind": "pdf", "page": page_index + 1, "xref": xref}
            caption = next((
                block["text"] for block in blocks
                if block.get("locator", {}).get("page") == page_index + 1
                and re.match(r"^(?:fig(?:ure)?\.?|table)\s+\d+", block.get("text", ""), re.I)
            ), "")
            assets.append({
                "block_index": None, "kind": "figure", "storage_path": storage_path,
                "source_url": "", "mime_type": stored_mime, "width": width, "height": height,
                "content_hash": digest, "caption": caption, "alt_text": "", "description": "",
                "locator": locator,
            })
        if rendered_pages < MAX_PDF_PAGE_RENDERS and page.get_drawings():
            pixmap = page.get_pixmap(matrix=fitz.Matrix(1.5, 1.5), alpha=False)
            saved = _save_image(pixmap.tobytes("png"), asset_dir, "png")
            if saved:
                storage_path, stored_mime, width, height, digest = saved
                if not any(asset["content_hash"] == digest for asset in assets):
                    locator = {"kind": "pdf", "page": page_index + 1, "render": "page"}
                    assets.append({
                        "block_index": None, "kind": "page_render", "storage_path": storage_path,
                        "source_url": "", "mime_type": stored_mime, "width": width, "height": height,
                        "content_hash": digest, "caption": f"Page {page_index + 1} visual layout",
                        "alt_text": "", "description": "", "locator": locator,
                    })
                    rendered_pages += 1
    pdf_doc.close()
    content_hash = hashlib.sha256(Path(file_path).read_bytes()).hexdigest()
    return NormalizedSource(
        title=title or Path(file_path).name,
        page_count=page_count,
        content_hash=content_hash,
        mime_type="application/pdf",
        blocks=blocks,
        chunks=chunks,
        assets=assets,
        extractor="pdfplumber+pymupdf",
        extractor_version=f"{package_version('pdfplumber')}+{package_version('PyMuPDF')}",
    )


async def extract_source(
    *, source_type: str, doc_id: str, word_limit: int, merge_bboxes,
    file_path: str = "", source_url: str = "", raw_text: str = "", title: str = "",
) -> NormalizedSource:
    if source_type == "web":
        return await extract_web(source_url, doc_id, word_limit)
    if source_type == "text":
        return extract_text(raw_text, title, word_limit)
    if source_type == "pdf":
        if not file_path or not os.path.exists(file_path):
            raise FileNotFoundError(f"file {file_path} not found")
        return await asyncio.to_thread(extract_pdf, file_path, doc_id, word_limit, merge_bboxes, title)
    raise ValueError(f"unsupported source type: {source_type}")
