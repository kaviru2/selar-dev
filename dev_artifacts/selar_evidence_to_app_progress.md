# SELAR evidence → working-app mission state

Updated: 2026-09-25 (+05:30). Current status: **in progress; not a participant-ready study or verified live stack**.

## Goal and stop condition

Deliver more than a requirements audit: versioned survey analysis where authorized responses exist; question→finding→requirement→implementation→test traceability; source-grounded current/prior passage suggestions prompting comparison/explanation/retrieval; human confirmation/correction and provenance-aware reversible graph actions in the **actual learner flow**; privacy/accessibility; test-first unit, integration and realistic synthetic E2E checks; green review-eligible merged PRs. Stop only when each applicable flow is actually exercised and all merged states and limitations recorded. Formal participant study launch, consent/cohort/outcome changes, recruitment, contact and disabled-reranker activation are excluded without separate approved protocol and gates.

## Phase 0 — source and access boundary

- Starting main: `99f217f53059dca3c7a136d9d23123e640f784de`; issues #9/#11/#14 open. Canonical Doc `1-MCai2wAhpRfy_R0O0TNggdo5NuYaPrthL-UgR6OlUs` read via authorized Docs API; it calls for suggested cross-document links confirmed by the learner.
- Form ID `19DrwsofX9ck0rxNHfpMxpW25JGVSzjxgXlusVQx0T-U`: public respondent-view question snapshot saved locally with SHA-256 `0a94942ed41f9bf52dba02a0419ae7007973dd6d0642fa50125933ed4af29e3e`; 33 question items plus four section headings. Drive modified timestamp `2026-09-17T15:35:29.925Z`. No response-row access or current count verified.
- Forms API: HTTP 403, API disabled in OAuth project. Drive search found no uniquely identified linked response Sheet. Isolated browser editor redirected to `viewform?edit_requested=true`. Authenticated Chrome foreground input required consent and timed out; do not retry it or use an equivalent bypass. The public questions can be analyzed, but actual respondent findings **cannot** yet be claimed.
- Traceability matrix: `dev_artifacts/formative_requirements_traceability_2026-09-25.md`; every respondent-derived finding is explicitly pending, including ambiguity/team review. No raw survey data/PII is committed.

## Phase 1 — traceability baseline / documentation PR

Status: merged PR #29 at `279a8804d374b1bca080103205633bbfec922d62`. The traceability matrix covers visible question groups; every respondent-derived finding remains pending response review. No current response rows were accessible, so this is not survey-results analysis.

## Phase 2 — source-grounded link to learner prompt

Status: implementation on `feat/grounded-candidate-contract` (PR/CI/merge not yet verified at this revision). Targeted RED→GREEN: worker witness `test_exact_two_sided_owner_matched_overlap_keeps_a_candidate` failed missing asserted concept then passed; asyncpg JSON locator failed then passed; Go evidence payload and unclassified constant failed compilation then passed; console evidence presenter failed missing module then passed. Full local checks: worker `55 passed, 2 skipped` (including live DB test skipped: local Postgres unavailable), Go `go test ./... -count=1` and `go vet ./...` passed, console Vitest `14 passed` and production `pnpm build` passed. CI's pgvector/Postgres worker job will exercise actual witness INSERT/trigger rejection if this branch reaches PR; no Docker installed or started here. The worker now keeps only exact two-sided overlap witnesses as a directed new→prior candidate; distance-only passage matches remain unclassified and dismissible, not confirmable. Neither the disabled reranker nor the frozen splits changed. Next: review CI DB boundary, record PR/merge SHA only after verified remote readback; then Phase 3 persisted review/correction/retraction. Limits: overlap-only conservative classifier, local live DB/service E2E unrun, historical ungrounded legacy records not upgraded, no participant study.

## Phase 3 — human review and graph corrections

Status: pending. Reuse offline graph reducer from PR #27 as a tested model; add persisted authorization and revision-bound preview/confirm/correct/retract/rollback semantics, with event/audit state and correction visibility. Do not treat `helpful` or generated answer text as source authority. TDD, integration and realistic synthetic end-to-end checks; green PR only.

## Phase 4 — privacy, accessibility, integrated rehearsal

Status: pending. Accessible evidence labels, keyboard flow, cross-user isolation, PII/consent honesty. Exercise registration (prototype only), upload/ingest/read, link suggestion, explanation/retrieval, confirm/correct, graph provenance/retraction with synthetic users and actual service boundaries where safely available. Docker is not installed/approved for unattended host changes; do not install or restart it remotely. Log blocked live-stack checks separately from executed tests. Merge only green review-eligible PRs.

## Notification and integrity rule

After **each verified merged milestone**, report in this Telegram channel: changed behavior, targeted/full/synthetic tests, CI, merge SHA, next phase and blocker. Keep this file current with real artifacts and final verification before claiming completion. Any inaccessible source remains explicitly `unverified`, not an invented metric. No email or participant contact.
