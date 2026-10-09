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

## Milestone 2 — reading checkpoints
Reader now gates source/connection display behind an optional warm-up, keeps the reading accessible via Continue reading, and offers an end-reading check. Practice drafts reset per document/phase; source quotes/reference answers appear only after submission. Immediate reading checks are marked exposed. Formal quiz components unchanged.
- `pnpm test components/ReadingPractice.test.tsx` → RED missing component (`m2-red.log`).
- `pnpm test` → passed (`m2-console.log`).
- `pnpm build` → passed (`m2-build.log`).

## Milestone 3 — daily review
Owner-scoped live-source due queue and one-at-a-time review page. Atomic attempt/schedule writes and concurrent idempotency-key regression. Conservative doubling-interval-v1 heuristic (not FSRS and not a recall model), bounded 1–60 days, resets for exposed/uncertain/low-score attempts. Explicit UTC date streak excludes future timestamps and deduplicates days. Production time is database time; tests age only fabricated rows.
- RED scheduler, DB due queue, UI: `m3-red.log`, `m3-db-red.log`, `m3-ui-red.log`.
- Full worker pytest with isolated Postgres → passed (`m3-worker.log`).
- Full console tests/build → receipts `m3-console.log`, `m3-build.log`.

## Milestone 4 — reflection links
Replaced reader keep/reject approval UI with locally drafted neutral reflection and comparison of live exact witnesses. Quiet flag uses existing revision-bound audit transaction to reject candidate or retract a historical saved link; it adds no assertion or recall success. Historical records, formal instruments and evidence files preserved. Added server cohort gate to direct witness preview/respond/flag, including known-ID locked-cohort real DB regression. Current reader/onboarding/chat/README/deployment copy updated; removed graph candidate promotion controls. Explicit user-authored graph correction remains a separate editing tool, not an automatic-link approval step.
- RED: `m4-go-red.log`, `m4-ui-red.log`, `m4-copy-red.log`.
- `TEST_DATABASE_URL=... go test ./...` → passed (`m4-go.log`).
- Console full regression/build: `m4-ui.log`, `m4-build.log`.

## Milestone 5 — truthful progress
Added owner-private current-source progress aggregates and `/progress`, plus Review/Progress navigation. Delayed practice requires server-observed 24h since previous attempt and no reported assistance; warm-up, immediate checks, exposed and unscored attempts are separate. UI discloses self-report and AI-score limitations, no mastery/efficacy claim, no invented zero-retention score. Estimated recall explicitly unavailable (not calibrated); streak is participation only; formal results remain a separate link.
- RED DB + UI/navigation: `m5-red.log`, `m5-ui-red.log`.
- Full worker/Postgres, console test and build pass: `m5-worker.log`, `m5-ui.log`, `m5-build.log`.

## Remaining milestones
6 pending full integrated acceptance testing. Provider tests use explicitly synthetic model fixtures; no production model quality, deployment or participant efficacy is claimed.
