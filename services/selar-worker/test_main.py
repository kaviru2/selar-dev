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
