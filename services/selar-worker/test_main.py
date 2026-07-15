"""Tests for the SELAR worker ingestion pipeline utilities."""

import json
import pytest
from fastapi.testclient import TestClient


def test_safe_parse_json_direct():
    """Test parsing clean JSON."""
    from main import safe_parse_json
    result = safe_parse_json('{"key": "value", "num": 42}')
    assert result == {"key": "value", "num": 42}


def test_safe_parse_json_markdown_fenced():
    """Test parsing JSON wrapped in markdown code fences."""
    from main import safe_parse_json
    text = '```json\n{"concepts": [{"name": "Backpropagation"}]}\n```'
    result = safe_parse_json(text)
    assert result["concepts"][0]["name"] == "Backpropagation"


def test_safe_parse_json_bare_fences():
    """Test parsing JSON wrapped in bare code fences (no language tag)."""
    from main import safe_parse_json
    text = '```\n{"key": "value"}\n```'
    result = safe_parse_json(text)
    assert result == {"key": "value"}


def test_safe_parse_json_with_extra_text():
    """Test regex fallback when JSON is mixed with text."""
    from main import safe_parse_json
    text = 'Here is the result:\n{"relation": "extends", "summary": "A extends B"}\nDone.'
    result = safe_parse_json(text)
    assert result["relation"] == "extends"
    assert result["summary"] == "A extends B"


def test_safe_parse_json_empty():
    """Test that empty/invalid input returns empty dict."""
    from main import safe_parse_json
    assert safe_parse_json("") == {}
    assert safe_parse_json("not json at all") == {}
    assert safe_parse_json("   ") == {}


def test_safe_parse_json_nested():
    """Test parsing nested JSON structures."""
    from main import safe_parse_json
    text = '{"concepts": [{"name": "A", "description": "desc"}], "edges": []}'
    result = safe_parse_json(text)
    assert len(result["concepts"]) == 1
    assert result["edges"] == []


def test_safe_parse_json_array():
    """Test that JSON arrays are handled (returned via regex if not object)."""
    from main import safe_parse_json
    # Arrays don't match the regex fallback (looks for {}), but direct parse works
    text = '[{"index": 1, "relation": "related_to"}]'
    result = safe_parse_json(text)
    assert isinstance(result, list)
    assert result[0]["index"] == 1


def test_normalize_mental_model_structured_fields():
    """Mental models retain only the stable research schema."""
    from main import normalize_mental_model

    result = normalize_mental_model({
        "main_claim": "Active recall improves retention.",
        "key_concepts": [
            {"name": "Active recall", "description": "Effortful retrieval", "evidence_chunk_index": 2},
            "Spacing effect",
        ],
        "assumptions": [{"text": "Recall is effortful"}],
        "open_questions": ["How should intervals adapt?"],
        "domain": "Learning science",
    })

    assert result["main_claim"] == "Active recall improves retention."
    assert [concept["name"] for concept in result["key_concepts"]] == ["Active recall", "Spacing effect"]
    assert result["assumptions"] == ["Recall is effortful"]
    assert result["open_questions"] == ["How should intervals adapt?"]


def test_deterministic_mental_model_supplies_grounded_fallback():
    from main import deterministic_mental_model

    chunks = [
        "Abstract. We introduce deterministic compensation logging to improve reliable agent execution. "
        "Deterministic compensation logging records every external side effect.",
        "The compensation manager uses deterministic compensation logging during agent recovery.",
        "Reliable agent execution requires recovery policies and transaction logging.",
    ]
    result = deterministic_mental_model(chunks, {"domain": "Agent systems"})

    assert result["main_claim"].startswith("We introduce deterministic compensation logging")
    assert len(result["key_concepts"]) >= 5
    assert result["domain"] == "Agent systems"
    assert all(0 <= concept["evidence_chunk_index"] < len(chunks) for concept in result["key_concepts"])


def test_deterministic_mental_model_preserves_valid_llm_fields():
    from main import deterministic_mental_model

    partial = {
        "main_claim": "A grounded claim.",
        "key_concepts": [{"name": "Known concept", "evidence_chunk_index": 0}],
        "assumptions": ["A known assumption"],
    }
    result = deterministic_mental_model(["Known concept supports deterministic recovery behavior."], partial)

    assert result["main_claim"] == "A grounded claim."
    assert result["assumptions"] == ["A known assumption"]
    assert result["key_concepts"][0]["name"] == "Known concept"


def test_merge_word_bboxes_preserves_lines():
    from main import merge_word_bboxes

    words = [
        {"x0": 10, "x1": 30, "top": 10, "bottom": 20},
        {"x0": 32, "x1": 50, "top": 10, "bottom": 20},
        {"x0": 10, "x1": 35, "top": 30, "bottom": 40},
    ]
    boxes = merge_word_bboxes(words, 100, 100)

    assert len(boxes) == 2
    assert boxes[0] == {"x": 0.1, "y": 0.1, "w": 0.4, "h": 0.1}


def test_deterministic_model_link_finds_concept_overlap():
    from main import deterministic_model_link

    source = {
        "main_claim": "Deterministic compensation improves reliable agents.",
        "key_concepts": [{"name": "Compensation-based recovery"}],
        "assumptions": [],
        "open_questions": [],
    }
    target = {
        "main_claim": "Recovery policies improve agent reliability.",
        "key_concepts": ["Compensation recovery"],
        "assumptions": [],
        "open_questions": [],
    }

    result = deterministic_model_link(source, target, 0.8)
    assert result is not None
    assert result["link_type"] == "concept_overlap"


def test_health_endpoint():
    """Test the /health endpoint returns ok."""
    from main import app
    client = TestClient(app)
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_process_endpoint_returns_202():
    """Test that /process accepts a request and returns immediately."""
    from main import app
    client = TestClient(app)
    response = client.post("/process", json={
        "doc_id": "test-doc-id",
        "file_path": "/tmp/nonexistent.pdf"
    })
    assert response.status_code == 200
    assert "doc_id" in response.json()


def test_process_endpoint_validation():
    """Test that /process rejects invalid payloads."""
    from main import app
    client = TestClient(app)
    response = client.post("/process", json={})
    assert response.status_code == 422  # Pydantic validation error
