"""Single source of truth for Gemini text generation (model + thinking).

Every text-generation call in the worker goes through ``generate_text`` so the
model name and the thinking setting cannot drift between call sites.

Defaults favour the lightest, cheapest stable model with thinking off:

- ``gemini-2.5-flash-lite`` is the cheapest stable text model on the Gemini API
  pricing page (USD 0.10 input / 0.40 output per 1M tokens) with no announced
  shutdown date. It is pinned rather than a ``-latest`` alias.
- Thinking is disabled (``thinking_budget=0``) for 2.5 Flash/Flash-Lite. Gemini 3+
  models take ``thinking_level`` instead (``thinking_budget=0`` is rejected by
  gemini-3.5-flash-lite), so they get the lowest level they support.

Override the model with ``GEMINI_TEXT_MODEL`` (for example
``gemini-3.1-flash-lite`` if 2.5 access is withdrawn) and the level with
``GEMINI_THINKING_LEVEL``. Embeddings are configured separately in ``main``.
"""
from __future__ import annotations

import os
from typing import Any, Optional

from google.genai import types

DEFAULT_TEXT_MODEL = "gemini-2.5-flash-lite"
TEXT_MODEL = (os.getenv("GEMINI_TEXT_MODEL") or "").strip() or DEFAULT_TEXT_MODEL
THINKING_LEVEL_OVERRIDE = (os.getenv("GEMINI_THINKING_LEVEL") or "").strip().lower()

# Gemini 3+ models whose lowest documented thinking level is "low", not "minimal".
_NO_MINIMAL_LEVEL = ("-pro", "gemini-3.8-flash")


def _name(model: str) -> str:
    return model.strip().lower().removeprefix("models/")


def thinking_config(model: Optional[str] = None) -> Optional[types.ThinkingConfig]:
    """The least thinking ``model`` allows (None for non-thinking models)."""
    name = _name(model or TEXT_MODEL)
    if name.startswith(("gemini-1.", "gemini-2.0")):
        return None  # these models do not think
    if name.startswith("gemini-2.5"):
        # 2.5 Pro cannot disable thinking; 128 is its documented minimum.
        return types.ThinkingConfig(thinking_budget=128 if "-pro" in name else 0)
    level = THINKING_LEVEL_OVERRIDE or ("low" if any(tag in name for tag in _NO_MINIMAL_LEVEL) else "minimal")
    return types.ThinkingConfig(thinking_level=level)


def text_config(**kwargs: Any) -> types.GenerateContentConfig:
    """GenerateContentConfig with minimal thinking for the configured model."""
    return types.GenerateContentConfig(thinking_config=thinking_config(), **kwargs)


def generate_text(client: Any, contents: Any, **kwargs: Any) -> Any:
    """The only Gemini text-generation entry point in the worker."""
    return client.models.generate_content(model=TEXT_MODEL, contents=contents, config=text_config(**kwargs))
