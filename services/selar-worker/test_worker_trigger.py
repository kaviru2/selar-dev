"""Serverless (Modal) trigger and sweep tests. No Modal account or network.

Auth logic is pure. Queue tests reuse the real durable queue SQL against
TEST_DATABASE_URL and are skipped without it.
"""

import asyncio
import os
import uuid

import pytest
from fastapi.testclient import TestClient

import worker_trigger
from worker_trigger import SECRET_HEADER, secret_matches


def test_secret_matches_uses_exact_constant_time_comparison():
    assert secret_matches("a-long-shared-secret", "a-long-shared-secret") is True
    assert secret_matches("a-long-shared-secret", "a-long-shared-secreT") is False
    assert secret_matches("a-long-shared-secret", "") is False
    assert secret_matches("a-long-shared-secret", None) is False
    # An unset or empty configured secret must never authorise anything.
    assert secret_matches("", "") is False
    assert secret_matches(None, "anything") is False


def test_secret_comparison_uses_hmac_compare_digest(monkeypatch):
    calls = []
    real = worker_trigger.hmac.compare_digest

    def spy(a, b):
        calls.append((a, b))
        return real(a, b)

    monkeypatch.setattr(worker_trigger.hmac, "compare_digest", spy)
    secret_matches("a-long-shared-secret", "guess")
    assert calls, "secret comparison must go through hmac.compare_digest"


def _app(monkeypatch, secret="a-long-shared-secret", processed=None):
    processed = processed if processed is not None else []

    async def fake_process_job(job_id):
        processed.append(job_id)
        return "completed"

    monkeypatch.setenv("WORKER_TRIGGER_SECRET", secret)
    return worker_trigger.build_trigger_app(process_job=fake_process_job, spawn=None), processed


def test_trigger_endpoint_requires_the_shared_secret(monkeypatch):
    app, processed = _app(monkeypatch)
    client = TestClient(app)
    job_id = str(uuid.uuid4())
    assert client.post("/jobs/trigger", json={"job_id": job_id}).status_code == 401
    assert client.post("/jobs/trigger", json={"job_id": job_id}, headers={SECRET_HEADER: "wrong"}).status_code == 401
    assert processed == []
    accepted = client.post("/jobs/trigger", json={"job_id": job_id}, headers={SECRET_HEADER: "a-long-shared-secret"})
    assert accepted.status_code == 202
    assert processed == [job_id]


def test_trigger_endpoint_fails_closed_without_configured_secret(monkeypatch):
    app, processed = _app(monkeypatch, secret="")
    response = TestClient(app).post("/jobs/trigger", json={"job_id": str(uuid.uuid4())}, headers={SECRET_HEADER: ""})
    assert response.status_code == 503
    assert processed == []


def test_trigger_endpoint_validates_job_id(monkeypatch):
    app, processed = _app(monkeypatch)
    client = TestClient(app)
    for bad in ({"job_id": "../../etc"}, {"job_id": ""}, {}):
        response = client.post("/jobs/trigger", json=bad, headers={SECRET_HEADER: "a-long-shared-secret"})
        assert response.status_code == 422
    assert processed == []


def test_trigger_endpoint_spawns_when_a_spawner_is_provided(monkeypatch):
    spawned = []
    monkeypatch.setenv("WORKER_TRIGGER_SECRET", "a-long-shared-secret")

    async def never(job_id):  # pragma: no cover - must not run inline
        raise AssertionError("spawned jobs must not run inline")

    app = worker_trigger.build_trigger_app(process_job=never, spawn=spawned.append)
    job_id = str(uuid.uuid4())
    response = TestClient(app).post("/jobs/trigger", json={"job_id": job_id}, headers={SECRET_HEADER: "a-long-shared-secret"})
    assert response.status_code == 202 and spawned == [job_id]


def test_health_endpoint_needs_no_secret(monkeypatch):
    app, _ = _app(monkeypatch)
    assert TestClient(app).get("/health").json() == {"status": "ok"}


def test_chat_route_requires_secret_when_configured(monkeypatch):
    """When WORKER_TRIGGER_SECRET is set (serverless), /chat is not public."""
    app, _ = _app(monkeypatch)
    response = TestClient(app).post("/chat", json={"user_id": str(uuid.uuid4()), "question": "q"})
    assert response.status_code == 401


def test_modal_app_imports_without_modal_installed():
    import importlib

    module = importlib.import_module("modal_app")
    assert module.MODAL_AVAILABLE in (True, False)
    assert module.SECRET_NAMES == ["selar-worker-secrets"]
    assert module.SWEEP_SCHEDULE_MINUTES >= 1


# --- Durable queue semantics (Postgres) ------------------------------------

needs_db = pytest.mark.skipif(not os.getenv("TEST_DATABASE_URL"), reason="TEST_DATABASE_URL is not configured")


async def _seed_job(conn, *, status="queued", attempts=0, available_offset="0 seconds", lease_offset=None):
    user_id = await conn.fetchval(
        "INSERT INTO users (email, password_hash) VALUES ($1, 'test') RETURNING id", f"modal-{uuid.uuid4()}@example.test")
    source_id = await conn.fetchval(
        "INSERT INTO content_sources (user_id, kind, title) VALUES ($1, 'text', 'Synthetic') RETURNING id", user_id)
    document_id = await conn.fetchval(
        """INSERT INTO documents (user_id, title, status, source_id, source_type)
           VALUES ($1, 'Synthetic', 'processing', $2, 'text') RETURNING id""", user_id, source_id)
    run_id = await conn.fetchval(
        "INSERT INTO ingestion_runs (source_id, document_id, user_id) VALUES ($1, $2, $3) RETURNING id",
        source_id, document_id, user_id)
    job_id = await conn.fetchval(
        f"""INSERT INTO ingestion_jobs (run_id, source_id, document_id, user_id, source_type, raw_text, title,
               status, attempts, available_at, lease_expires_at, worker_id)
           VALUES ($1, $2, $3, $4, 'text', 'Synthetic evidence.', 'Synthetic', $5, $6,
                   now() + interval '{available_offset}',
                   {"now() + interval '" + lease_offset + "'" if lease_offset else 'NULL'},
                   CASE WHEN $5 = 'leased' THEN 'other-worker' ELSE '' END)
           RETURNING id""",
        run_id, source_id, document_id, user_id, status, attempts)
    return user_id, job_id


@needs_db
def test_claim_specific_job_leases_once_and_refuses_double_processing(monkeypatch):
    import asyncpg
    import main

    database_url = os.environ["TEST_DATABASE_URL"]
    monkeypatch.setattr(main, "DATABASE_URL", database_url)

    async def exercise():
        conn = await asyncpg.connect(database_url)
        user_id, job_id = await _seed_job(conn)
        _, delayed_id = await _seed_job(conn, available_offset="10 minutes")
        _, busy_id = await _seed_job(conn, status="leased", attempts=1, lease_offset="5 minutes")
        try:
            first, second = await asyncio.gather(main.claim_ingestion_job(str(job_id)), main.claim_ingestion_job(str(job_id)))
            claimed = [job for job in (first, second) if job]
            assert len(claimed) == 1, "concurrent triggers must lease a job exactly once"
            assert claimed[0]["status"] == "leased" and claimed[0]["attempts"] == 1
            assert await main.claim_ingestion_job(str(job_id)) is None
            # A trigger is a hint, not an override: backoff and live leases are respected.
            assert await main.claim_ingestion_job(str(delayed_id)) is None
            assert await main.claim_ingestion_job(str(busy_id)) is None
        finally:
            await conn.execute("DELETE FROM users WHERE id = $1", user_id)
            await conn.execute("DELETE FROM users WHERE id IN (SELECT user_id FROM ingestion_jobs WHERE id = ANY($1::uuid[]))",
                               [delayed_id, busy_id])
            await conn.close()

    asyncio.run(exercise())


@needs_db
def test_process_job_runs_claimed_job_through_normal_completion(monkeypatch):
    import asyncpg
    import main

    database_url = os.environ["TEST_DATABASE_URL"]
    monkeypatch.setattr(main, "DATABASE_URL", database_url)
    seen = []

    async def fake_task(**kwargs):
        seen.append(kwargs["doc_id"])

    monkeypatch.setattr(main, "process_document_task", fake_task)

    async def exercise():
        conn = await asyncpg.connect(database_url)
        user_id, job_id = await _seed_job(conn)
        try:
            assert await worker_trigger.process_job(str(job_id)) == "completed"
            assert await worker_trigger.process_job(str(job_id)) == "not_claimable"
            assert await conn.fetchval("SELECT status FROM ingestion_jobs WHERE id = $1", job_id) == "completed"
            assert len(seen) == 1
        finally:
            await conn.execute("DELETE FROM users WHERE id = $1", user_id)
            await conn.close()

    asyncio.run(exercise())


@needs_db
def test_sweep_requeues_expired_leases_and_drains_ready_jobs(monkeypatch):
    import asyncpg
    import main

    database_url = os.environ["TEST_DATABASE_URL"]
    monkeypatch.setattr(main, "DATABASE_URL", database_url)
    processed = []

    async def fake_task(**kwargs):
        processed.append(kwargs["doc_id"])

    monkeypatch.setattr(main, "process_document_task", fake_task)

    async def exercise():
        conn = await asyncpg.connect(database_url)
        # Isolate from other rows: finish anything already ready.
        await conn.execute("UPDATE ingestion_jobs SET status = 'cancelled' WHERE status IN ('queued', 'leased')")
        owner_a, stale = await _seed_job(conn, status="leased", attempts=1, lease_offset="-1 minutes")
        owner_b, ready = await _seed_job(conn)
        owner_c, later = await _seed_job(conn, available_offset="10 minutes")
        try:
            result = await worker_trigger.sweep(max_jobs=10)
            assert result["processed"] == 2
            statuses = {row["id"]: row["status"] for row in await conn.fetch(
                "SELECT id, status FROM ingestion_jobs WHERE id = ANY($1::uuid[])", [stale, ready, later])}
            assert statuses[stale] == "completed" and statuses[ready] == "completed"
            assert statuses[later] == "queued", "backoff must be respected by the sweep"
        finally:
            await conn.execute("DELETE FROM users WHERE id = ANY($1::uuid[])", [owner_a, owner_b, owner_c])
            await conn.close()

    asyncio.run(exercise())
