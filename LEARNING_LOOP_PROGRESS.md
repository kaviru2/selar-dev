# Learning-loop implementation progress

Issues: #135 generation; #136 reading checkpoints; #137 review queue; #138 learning links; #139 truthful progress; #140 synthetic E2E. No issue is auto-closed; no PR is merged by this implementation mission.

## Safety boundaries
Practice tables and AI grading are separate from fixed admin study instruments. Exact quotes and separate AI checks do not establish truth or mastery. No participant activity, emails, slides, frozen splits, research-lock changes, deployment, or changes to relationship-reranker settings. Existing PRs #133/#134 untouched.

## Milestone 1 — grounded private practice
Implemented owner-private practice generation after ingestion and explicit retry, exact located source witness + independent AI answer-grounding call, private free-recall grading, bounded finite scoring with uncertain/unavailable feedback unscored, idempotent attempts, live source hash/chunk/locator validation, authenticated owner-bound API RPC, Modal packaging.

Receipts are retained outside the repository at `/Users/kavirum1a/.hermes/cache/scratch/selar-loop-receipts`.
- RED: `../../.venv/bin/python -m pytest test_practice.py -q` → missing practice module; `test_practice_service.py` → missing service; attempt slice → missing attempt function. Logs `m1-red.log`, `m1-db-red.log`, `m1-attempt-red.log`.
- Real isolated PostgreSQL 17 + pgvector, port **55447**, database `selar_e2e`; existing 55439 was not used. Locale C required to start local Postgres.
- `DATABASE_URL=postgres://selar_e2e@127.0.0.1:55447/selar_e2e?sslmode=disable go run ./cmd/migrate` → migration 021 applied on prior 020 database (`m1-migrate.log`).
- `TEST_DATABASE_URL=postgres://selar_e2e@127.0.0.1:55447/selar_e2e?sslmode=disable go test ./...` → passed (`m1-go.log`).
- Worker regression receipt `m1-worker.log`; service/browser fixtures require dedicated E2E invocation, not counted as executed when skipped.

## Remaining milestones
2–6 pending implementation and full integrated acceptance testing. Provider tests use explicitly synthetic model fixtures; no production model quality, deployment or participant efficacy is claimed.
