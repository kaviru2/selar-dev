"""Modal deployment of the SELAR worker (serverless alternative to the poller).

Deploy (from services/selar-worker, after creating the Modal secret):

    modal deploy modal_app.py

What it defines:

- ``web``: an ASGI web endpoint exposing the worker routes (/health, /chat,
  /process) plus ``POST /jobs/trigger``. Every route except /health needs the
  ``X-Selar-Worker-Secret`` header (constant-time compared with
  WORKER_TRIGGER_SECRET). Set the API's WORKER_URL to this endpoint's URL.
- ``process_job``: runs one durable ingestion job. The trigger spawns it so the
  HTTP call returns immediately; it claims the job with the same lease SQL as
  the polling worker, so duplicates are harmless.
- ``sweep``: scheduled every SWEEP_SCHEDULE_MINUTES. It re-queues expired
  leases and drains ready jobs (retries after backoff, missed triggers).
- ``notifications``: scheduled every NOTIFY_SCHEDULE_MINUTES. It asks the API
  to run the optional email study notices (dry-run unless switched on there)
  and does nothing unless SELAR_API_URL is set in the secret.

Secrets are referenced by name only. Create one Modal secret named
``selar-worker-secrets`` with the keys listed in docs/DEPLOYMENT.md.

``modal`` is optional: without it this module still imports (for tests and
local tooling) and only the pure configuration below is available.
"""

from __future__ import annotations

import asyncio

try:  # pragma: no cover - depends on the deploy environment
    import modal

    MODAL_AVAILABLE = True
except ImportError:  # pragma: no cover
    modal = None
    MODAL_AVAILABLE = False

APP_NAME = "selar-worker"
SECRET_NAMES = ["selar-worker-secrets"]
SWEEP_SCHEDULE_MINUTES = 5
NOTIFY_SCHEDULE_MINUTES = 15
# Lease (INGESTION_LEASE_SECONDS, default 300) is renewed by the heartbeat; the
# function timeout bounds a single large PDF.
JOB_TIMEOUT_SECONDS = 30 * 60
SWEEP_TIMEOUT_SECONDS = 15 * 60
LOCAL_MODULES = ["main", "ingestion", "storage", "worker_trigger", "candidate_contract", "candidate_generation",
                 "offline_evidence", "evaluation", "notify_schedule", "practice", "practice_service", "practice_routes", "practice_schedule"]

if MODAL_AVAILABLE:  # pragma: no cover - exercised only on Modal
    image = (
        modal.Image.debian_slim(python_version="3.11")
        .pip_install_from_requirements("requirements.txt")
        # The serverless app never runs the in-process poller.
        .env({"INGESTION_QUEUE_ENABLED": "false"})
        .add_local_python_source(*LOCAL_MODULES)
    )
    app = modal.App(APP_NAME, image=image)
    secrets = [modal.Secret.from_name(name) for name in SECRET_NAMES]

    @app.function(secrets=secrets, timeout=JOB_TIMEOUT_SECONDS)
    def process_job(job_id: str) -> str:
        import worker_trigger

        return asyncio.run(worker_trigger.process_job(job_id))

    @app.function(secrets=secrets, timeout=SWEEP_TIMEOUT_SECONDS,
                  schedule=modal.Period(minutes=SWEEP_SCHEDULE_MINUTES))
    def sweep() -> dict:
        import worker_trigger

        return asyncio.run(worker_trigger.sweep(time_budget_seconds=SWEEP_TIMEOUT_SECONDS - 120))

    @app.function(secrets=secrets, timeout=120,
                  schedule=modal.Period(minutes=NOTIFY_SCHEDULE_MINUTES))
    def notifications() -> dict:
        import notify_schedule

        return notify_schedule.run_notifications()

    @app.function(secrets=secrets, timeout=120)
    @modal.concurrent(max_inputs=20)
    @modal.asgi_app()
    def web():
        import worker_trigger

        return worker_trigger.build_trigger_app(spawn=lambda job_id: process_job.spawn(job_id))
