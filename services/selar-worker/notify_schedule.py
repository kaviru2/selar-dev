"""Scheduled trigger for SELAR's optional email study notices (issue #114).

Modal runs ``run_notifications`` every NOTIFY_SCHEDULE_MINUTES. It only makes
one authenticated POST to the API's ``/internal/notifications/run``; the API
decides what is due, enforces opt-ins, idempotency and rate limits, and by
default records every notice as dry-run without sending anything.

The call is skipped unless SELAR_API_URL is set in the Modal secret, so this
schedule does nothing until the research team configures it.
"""

from __future__ import annotations

import os
from typing import Any, Optional

import httpx

SECRET_HEADER = "X-Selar-Worker-Secret"
RUN_PATH = "/internal/notifications/run"


def run_notifications(
    api_url: Optional[str] = None,
    secret: Optional[str] = None,
    client: Optional[httpx.Client] = None,
    timeout: float = 60.0,
) -> dict[str, Any]:
    """POST to the API's notification run. Returns the API's report.

    Never raises for HTTP errors: the next scheduled run simply tries again,
    and idempotency on the API side makes retries harmless.
    """
    api_url = (api_url if api_url is not None else os.getenv("SELAR_API_URL", "")).strip().rstrip("/")
    secret = secret if secret is not None else os.getenv("WORKER_TRIGGER_SECRET", "")
    if not api_url:
        return {"skipped": "SELAR_API_URL not set"}
    if not secret:
        return {"skipped": "WORKER_TRIGGER_SECRET not set"}
    if not api_url.startswith("https://") and not api_url.startswith("http://localhost"):
        return {"skipped": "SELAR_API_URL must be https"}
    own = client is None
    client = client or httpx.Client(timeout=timeout)
    try:
        resp = client.post(api_url + RUN_PATH, headers={SECRET_HEADER: secret})
        if resp.status_code != 200:
            return {"error": f"HTTP {resp.status_code}"}
        return resp.json()
    except httpx.HTTPError as exc:
        return {"error": type(exc).__name__}
    finally:
        if own:
            client.close()
