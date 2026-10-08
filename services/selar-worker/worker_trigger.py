"""Serverless entry points for the durable ingestion queue (used by modal_app.py).

- ``build_trigger_app`` is an ASGI app that serves the worker routes and adds
  an authenticated ``POST /jobs/trigger``. The Go API calls it after it commits
  a job. Every route except ``/health`` requires the ``X-Selar-Worker-Secret``
  header, which is compared in constant time against WORKER_TRIGGER_SECRET.
  If no secret is configured the app fails closed.
- ``process_job`` claims one specific job with the same lease SQL the polling
  worker uses, so a duplicate trigger, a concurrent sweep or a local poller can
  never process a job twice.
- ``sweep`` re-queues expired leases and drains ready jobs. A scheduled sweep
  makes a lost trigger harmless.

Nothing here imports Modal, so the normal test suite runs without it.
"""

from __future__ import annotations

import hmac
import inspect
import os
import time
import uuid
from typing import Any, Awaitable, Callable, Optional

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel, field_validator

import main

SECRET_HEADER = "X-Selar-Worker-Secret"
PUBLIC_PATHS = {"/health"}


def secret_matches(configured: Optional[str], provided: Optional[str]) -> bool:
    """Constant-time comparison; an empty configured secret never matches."""
    if not configured or provided is None:
        return False
    return hmac.compare_digest(configured.encode("utf-8"), provided.encode("utf-8"))


class TriggerRequest(BaseModel):
    job_id: str

    @field_validator("job_id")
    @classmethod
    def _uuid(cls, value: str) -> str:
        return str(uuid.UUID(value))


async def process_job(job_id: str) -> str:
    """Claim and run one job. Returns 'completed' or 'not_claimable'."""
    job = await main.claim_ingestion_job(job_id)
    if not job:
        return "not_claimable"
    await main.run_claimed_ingestion_job(job)
    return "completed"


async def sweep(max_jobs: int = 20, time_budget_seconds: float = 600) -> dict[str, int]:
    """Re-queue expired leases and process ready jobs until none remain or the budget ends."""
    started = time.monotonic()
    processed = 0
    while processed < max_jobs and time.monotonic() - started < time_budget_seconds:
        job = await main.claim_ingestion_job()
        if not job:
            break
        await main.run_claimed_ingestion_job(job)
        processed += 1
    return {"processed": processed}


def build_trigger_app(
    process_job: Callable[[str], Awaitable[Any]] = process_job,
    spawn: Optional[Callable[[str], Any]] = None,
) -> FastAPI:
    """Worker routes plus /jobs/trigger, all behind the shared secret.

    ``spawn`` hands the job to a separate function call (Modal ``.spawn``) so
    the HTTP response returns immediately. Without it the job runs inline.
    """
    app = FastAPI(title="SELAR serverless worker")

    @app.middleware("http")
    async def require_secret(request: Request, call_next):
        if request.url.path in PUBLIC_PATHS:
            return await call_next(request)
        configured = os.getenv("WORKER_TRIGGER_SECRET", "")
        if not configured:
            return JSONResponse({"error": "worker secret not configured"}, status_code=503)
        if not secret_matches(configured, request.headers.get(SECRET_HEADER)):
            return JSONResponse({"error": "unauthorized"}, status_code=401)
        return await call_next(request)

    @app.post("/jobs/trigger", status_code=202)
    async def trigger(req: TriggerRequest):
        if spawn is not None:
            result = spawn(req.job_id)
            if inspect.isawaitable(result):
                await result
            return {"job_id": req.job_id, "status": "spawned"}
        status = await process_job(req.job_id)
        return {"job_id": req.job_id, "status": status}

    # /health, /chat and /process from the regular worker.
    app.include_router(main.app.router)
    return app
