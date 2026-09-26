import hashlib

import fitz
import pdfplumber
import pytest

from evaluation import evaluate, ndcg_at_k, recall_at_k, reciprocal_rank
from ingestion import (
    UnsafeSourceURL,
    _is_public_ip,
    canonicalize_url,
    extract_text,
    extract_pdf,
    is_likely_decorative_image,
    is_weak_image_alt,
    markdown_blocks,
)
from main import merge_word_bboxes


def test_canonicalize_url_removes_fragment_and_normalizes_host():
    assert canonicalize_url("HTTPS://Example.COM/paper?q=1#results") == "https://example.com/paper?q=1"


def test_canonicalize_url_matches_api_rules_for_root_default_port_and_tracking():
    assert canonicalize_url("https://Example.COM:443?utm_source=test&keep=1") == "https://example.com/?keep=1"


@pytest.mark.parametrize("value", ["file:///etc/passwd", "javascript:alert(1)", "http://"])
def test_canonicalize_url_rejects_unsafe_schemes(value):
    with pytest.raises(UnsafeSourceURL):
        canonicalize_url(value)


@pytest.mark.parametrize("address", ["127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fc00::1"])
def test_private_addresses_are_blocked(address):
    assert not _is_public_ip(address)


def test_markdown_blocks_preserve_heading_and_structure():
    blocks = markdown_blocks("# Main claim\n\nA grounded paragraph.\n\n- Supporting evidence")
    assert [block["kind"] for block in blocks] == ["heading", "paragraph", "list"]
    assert [block["block_index"] for block in blocks] == [0, 1, 2]


def test_text_adapter_keeps_stable_locators():
    result = extract_text("# Topic\n\nThis paragraph contains evidence for the topic.", "Study note", 120)
    assert result.title == "Study note"
    assert result.chunks[0]["locator"]["block_index"] == 1
    assert result.chunks[0]["locator"]["heading"] == "Topic"
    assert len(result.content_hash) == 64


def test_pdf_line_end_hyphen_never_joins_unverified_word_across_chunks(tmp_path):
    path = tmp_path / "invented-wrap.pdf"
    document = fitz.open()
    page = document.new_page(width=400, height=240)
    page.insert_text((40, 60), "The co-")
    page.insert_text((40, 76), "operation remains documented.")
    page.insert_text((40, 100), "A co-operation is separately recorded.")
    document.save(path)
    document.close()

    with pdfplumber.open(path) as pdf:
        source_words = pdf.pages[0].extract_words(use_text_flow=True, x_tolerance=1, y_tolerance=3)
    assert [word["text"] for word in source_words] == [
        "The", "co-", "operation", "remains", "documented.",
        "A", "co-operation", "is", "separately", "recorded.",
    ]

    result = extract_pdf(str(path), "invented-document", 120, merge_word_bboxes)
    assert result.content_hash == hashlib.sha256(path.read_bytes()).hexdigest()
    assert [chunk["text"] for chunk in result.chunks] == [
        "The co-", "operation remains documented. A co-operation is separately recorded.",
    ]
    assert " ".join(chunk["text"] for chunk in result.chunks) == " ".join(word["text"] for word in source_words)
    assert [block["text"] for block in result.blocks] == [chunk["text"] for chunk in result.chunks]
    assert [chunk["locator"] for chunk in result.chunks] == [
        {"kind": "pdf", "page": 1, "block_index": index} for index in range(2)
    ]
    assert all(chunk["bboxes"] and chunk["page"] == 1 for chunk in result.chunks)


def test_decorative_images_are_excluded_but_captioned_figures_are_kept():
    assert is_likely_decorative_image("https://cdn.example/avatars/me.jpg", "Author avatar", "")
    assert not is_likely_decorative_image(
        "https://cdn.example/figure-2.png", "Architecture diagram", "Figure 2: Retrieval pipeline"
    )
    assert is_weak_image_alt("image/png")
    assert not is_weak_image_alt("Retrieval architecture")


def test_retrieval_metrics_are_deterministic_and_stratified():
    rows = [{
        "query_id": "q1",
        "answer_modality": "figure",
        "relevance": {"asset-a": 2.0, "asset-b": 1.0},
        "ranked_ids": ["noise", "asset-a", "asset-b"],
    }]
    report = evaluate(rows, (1, 3))
    assert report["all"]["mrr"] == 0.5
    assert report["figure"] == report["all"]
    assert recall_at_k({"asset-a"}, ["asset-a"], 1) == 1.0
    assert reciprocal_rank({"asset-a"}, ["noise", "asset-a"]) == 0.5
    assert 0 < ndcg_at_k(rows[0]["relevance"], rows[0]["ranked_ids"], 3) < 1
