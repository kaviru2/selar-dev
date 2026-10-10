"""Gemini text generation is centralised: one model, minimal thinking, fewer calls."""
import ast
import asyncio
import importlib
import json
from pathlib import Path
from types import SimpleNamespace

import pytest

HERE = Path(__file__).parent


def _reload(monkeypatch, **env):
    for name in ("GEMINI_TEXT_MODEL", "GEMINI_THINKING_LEVEL"):
        monkeypatch.delenv(name, raising=False)
    for name, value in env.items():
        monkeypatch.setenv(name, value)
    import genai_config
    return importlib.reload(genai_config)


@pytest.fixture(autouse=True)
def _restore_default_config(monkeypatch):
    yield
    monkeypatch.delenv("GEMINI_TEXT_MODEL", raising=False)
    monkeypatch.delenv("GEMINI_THINKING_LEVEL", raising=False)
    import genai_config
    importlib.reload(genai_config)


def test_default_is_pinned_cheapest_lite_model_with_thinking_off(monkeypatch):
    config = _reload(monkeypatch)
    assert config.TEXT_MODEL == "gemini-2.5-flash-lite"
    assert "preview" not in config.TEXT_MODEL and "latest" not in config.TEXT_MODEL
    thinking = config.text_config().thinking_config
    assert thinking.thinking_budget == 0 and thinking.thinking_level is None


@pytest.mark.parametrize("model,budget,level", [
    ("gemini-2.5-flash-lite", 0, None),
    ("models/gemini-2.5-flash", 0, None),
    ("gemini-2.5-pro", 128, None),
    ("gemini-3.1-flash-lite", None, "MINIMAL"),
    ("gemini-3.5-flash-lite", None, "MINIMAL"),
    ("models/gemini-3-flash-preview", None, "MINIMAL"),
    ("gemini-3.8-flash", None, "LOW"),
    ("gemini-3.1-pro-preview", None, "LOW"),
])
def test_every_model_family_gets_its_lowest_supported_thinking(monkeypatch, model, budget, level):
    config = _reload(monkeypatch, GEMINI_TEXT_MODEL=model)
    assert config.TEXT_MODEL == model
    thinking = config.thinking_config()
    assert thinking.thinking_budget == budget
    assert (thinking.thinking_level.value if thinking.thinking_level else None) == level


def test_non_thinking_models_send_no_thinking_config(monkeypatch):
    assert _reload(monkeypatch, GEMINI_TEXT_MODEL="gemini-2.0-flash-lite").thinking_config() is None


def test_blank_override_falls_back_to_default(monkeypatch):
    assert _reload(monkeypatch, GEMINI_TEXT_MODEL="  ").TEXT_MODEL == "gemini-2.5-flash-lite"


def test_generate_text_sends_configured_model_and_minimal_thinking(monkeypatch):
    config = _reload(monkeypatch)
    calls = []
    client = SimpleNamespace(models=SimpleNamespace(
        generate_content=lambda **kwargs: calls.append(kwargs) or SimpleNamespace(text="{}")))
    config.generate_text(client, "hello", response_mime_type="application/json")
    assert calls[0]["model"] == config.TEXT_MODEL
    assert calls[0]["config"].thinking_config.thinking_budget == 0
    assert calls[0]["config"].response_mime_type == "application/json"


def test_no_module_calls_generate_content_except_the_central_helper():
    offenders = []
    for path in HERE.glob("*.py"):
        if path.name.startswith("test_") or path.name in {"genai_config.py", "synthetic_chat_worker.py"}:
            continue
        for node in ast.walk(ast.parse(path.read_text())):
            if isinstance(node, ast.Attribute) and node.attr == "generate_content":
                offenders.append(f"{path.name}:{node.lineno}")
            if isinstance(node, ast.Constant) and isinstance(node.value, str) and node.value.startswith(
                    ("gemini-", "models/gemini-")) and "embedding" not in node.value:
                offenders.append(f"{path.name}:{node.lineno} hard-coded {node.value}")
    assert offenders == []


def test_main_and_practice_share_the_one_model_name():
    import genai_config
    import main
    import practice_routes  # noqa: F401  (imports genai_config)
    assert main.TEXT_MODEL == genai_config.TEXT_MODEL == practice_routes.genai_config.TEXT_MODEL


def test_chat_and_practice_calls_use_minimal_thinking(monkeypatch):
    import main
    import practice_routes
    calls = []

    class Models:
        def generate_content(self, **kwargs):
            calls.append(kwargs)
            return SimpleNamespace(text=json.dumps({"score": 1, "confident": True, "feedback": "ok"}))

    monkeypatch.setattr(main, "client", SimpleNamespace(models=Models()))
    asyncio.run(practice_routes.model("grade", {"question": "Q", "answer": "A", "quote": "A", "response": "B"}))
    main.generate_mental_model_payload("[CHUNK 0]\nInvented text.\n")
    assert len(calls) == 2
    for call in calls:
        assert call["model"] == main.TEXT_MODEL
        assert call["config"].thinking_config.thinking_budget == 0


def test_mental_model_prompt_no_longer_requests_unpersisted_edges():
    import main
    assert "concept_edges" not in main.MENTAL_MODEL_PROMPT
    assert main.MENTAL_MODEL_PROMPT_VERSION == "mental-model-v2"


def test_practice_model_has_no_verify_task(monkeypatch):
    import main
    import practice_routes
    monkeypatch.setattr(main, "client", SimpleNamespace(models=SimpleNamespace(
        generate_content=lambda **_: SimpleNamespace(text="{}"))))
    with pytest.raises(KeyError):
        asyncio.run(practice_routes.model("verify", {}))
