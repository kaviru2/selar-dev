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

## Milestone 6 — integrated real-service acceptance
Added synthetic multipart PDF upload → durable ingestion → automatic generation → Chromium warm-up → source reading → end-reading check → test-only persisted day-N aging → daily review → progress. API, worker, Next, Chromium and isolated PostgreSQL/pgvector are real; only external model responses are synthetic. Browser borrower isolation, disabled-cohort direct witness routes, quiet flag/readback and zero graph/recall-success side effects are asserted. Existing PDF, web and chat service scenarios still run; historical review lifecycle remains an explicit API audit fixture rather than the removed reader UI.

Independent-review fixes included in this final integration slice:
- Warm-up does not create reading sessions or formal first-reading anchors; source-visible stage starts the session. Browser/DB assertion verifies zero before Continue reading and one afterward.
- Three source items generate concurrently with a 40-second total model budget and bounded lock waits. The real HTTP regression uses six synthetic 11-second provider calls and completes in **22.11 seconds**, without changing server timeouts. Budget exhaustion is unavailable, not a fabricated item.
- Wrong-link flags carry the displayed revision; stale flags return 409 without history mutation. Legacy similarity-response route also respects disabled cohorts.
- Changed extraction cannot overwrite an item and silently rebind old attempts. Delete/generation race, prior source invalidation, and no-reported-assistance plus recent in-app-reading exclusion are tested.
- Generation/verifier/grader model identifiers and policy versions retained. Injection/negation/partial-response tests verify the model boundary, not live-model accuracy.
- Upgrade 020→practice migrations preserves a submitted formal free-recall attempt, pending manual review, questions, rubric and feedback-never configuration byte-for-byte; clean migration and rerun tests also pass.

### Final local receipts
All paths below are under `/Users/kavirum1a/.hermes/cache/scratch/selar-loop-receipts`.
- `TEST_DATABASE_URL=postgres://selar_e2e@127.0.0.1:55447/selar_e2e?sslmode=disable go test -count=1 ./...` (API directory) → passed, `m6-go.log`.
- `go vet ./...` → passed, `m6-vet.log`.
- `TEST_DATABASE_URL=postgres://selar_e2e@127.0.0.1:55447/selar_e2e ../../.venv/bin/python -m pytest -q` (worker directory) → **153 passed, 7 dedicated-service skips**, `m6-worker.log`. Those skipped scenarios are exercised by the next command, not claimed executed here.
- `SELAR_SYNTHETIC_E2E=1 TEST_DATABASE_URL=postgres://selar_e2e@127.0.0.1:55447/selar_e2e TEST_API_BINARY=/Users/kavirum1a/.hermes/cache/scratch/selar-loop-receipts/selar-e2e-api TEST_CONSOLE_DIR=/Users/kavirum1a/.hermes/cache/scratch/selar-learning-loop/services/selar-console ../../.venv/bin/python -m pytest -vv -s test_synthetic_service_e2e.py test_chat_service_e2e.py test_web_service_e2e.py test_learning_loop_e2e.py` → **9 passed**, `m6-e2e.log`.
- `pnpm test` → **56 files / 376 tests passed**, `m6-ui.log`; `pnpm build` → passed, `m6-build.log`.
- Additional RED receipts: `m6-e2e-red.log`, `m6-session-red.log`, `m6-flags-red.log`, `m6-budget-red.log`, `m6-races-red.log`, `m6-exposure-red.log`, `m6-copy-red.log`.
- Existing PyMuPDF/SWIG/Starlette deprecation and React test-act warnings are visible in raw logs; no runtime failures are hidden.

### Publication and limitations
Six substantive stacked PRs; earlier milestone heads intentionally remain immutable. Final integration adds CI/coverage/synthetic workflow branch filters for `feat/learning-*` so full checks run against the integrated head while preserving milestone diffs. Remote head/check receipts are attached to PRs after publication; no earlier green run certifies a later head.

Not deployed. Live Gemini generation, entailment and grading quality are not validated by synthetic providers; they require deployment-side provider configuration/availability and separate evaluation. Practice generation is bounded synchronous work, not a new durable generation queue; unavailable/timeout offers retry. Schedule is an explicitly tested heuristic, not FSRS or a calibrated memory estimate. Streak days use UTC, not inferred local timezone. No participant/study readiness or efficacy is claimed. Parent must review/merge the full stack; no PR merged and no issue closed. Existing PR #133 may need UI reconciliation, #134 remains independent.
