# Persistent PDF highlights — issue #91

## Scope

- Selection stores the actual text-layer quote, prefix/suffix, UTF-16 start/end offsets, source content hash and normalised rectangle fallback. It never creates a quote from model output.
- Re-render/zoom resolves the complete quote with context. Source mismatch paints no mark and cannot edit/delete it; stored content is retained. Rectangle-only rows retain their existing representation and remain editable.
- Selection color, exact-text copy and a labeled inline note editor replace `window.prompt`. Editor autofocus, Escape/cancel, pending state and retryable errors are covered.
- Highlights list supports page/quote jump, copy, color/note edits and explicit delete confirmation. Presentation edits cannot replace page, quote, source, owner, chunk or bounding boxes.
- Owner-scoped PATCH; DELETE reuses the existing endpoint. Creation also rejects a chunk from another owner/document before writing annotation or learning-event rows.
- No acceptance/rejection workflow, research data, mail, production deploy or automatic merge/issue closure.

## Dependency and migration

The annotation work incorporates the complete format dependency **8e89027776df63e2193b0ddb7b639de43fc5aa11** (PR #158, including #157) and main **884bb9698b9a8ffb00ab1876fe3315e7eef1e228**. Changes authored for this task are restricted to annotation API/store/model, PDF annotation controls/helpers, minimal reader callbacks and their tests.

**024_annotation_text_anchors.sql** adds nullable `annotations.anchor`. Migration 023 belongs to the format dependency. Fresh database validation applied **001–024**, consecutively. Do not deploy 024 independently of 023. An early disposable test database had 024 before the format dependency arrived; the migration runner correctly refused its out-of-order history. Final suites and browser checks use a new database migrated in order, not a bypass of that guard.

## Executed local checks

- Go: `TEST_DATABASE_URL=<local fresh pgvector database> go test -json ./...` — **366 passed test/subtest result events, 0 failed, 0 skipped**; `go vet ./...` and API build passed. Counts include subtests, not 366 independent top-level tests.
- Console: `NODE_OPTIONS=--no-experimental-webstorage pnpm test` — **62 files, 415 tests passed**, no skips.
- `pnpm build` — successful production compilation and TypeScript build.
- ESLint for changed reader components, annotation helpers/tests and both browser scripts — passed.
- Real Chrome + built Next console + Go API + local PostgreSQL: `scripts/reader-browser-check.cjs` — **29/29**.
- Same stack: `scripts/annotation-browser-check.cjs` — **16/16**, including exact copy, note/color persistence, Escape cancellation, a deliberately injected failed PATCH followed by successful retry, readback after edits, reload, list jump, zoom/reflow, cancel/confirm deletion and deletion readback.
- Store tests verify cross-owner chunk tampering, owner-scoped updates/deletes, source-change rejection without mutation and legacy rectangle editing. Handler tests reject identity/anchor/geometry edits and malformed anchors. DOM helper tests cover nested find markup, cross-page selection exclusion and current-layout geometry.
- Tests were introduced before implementation. Observed failures included discarded anchors, accepted foreign chunk, accepted source mismatch, missing editor/list and absent current-layout geometry.

Raw local console/build/migration/browser receipts are in `highlights-test-logs/`. Their authentic carriage returns/trailing blank lines are preserved; `git diff --check` is clean for source/docs with only that raw-log directory excluded. Full Go JSON receipt remains in the isolated workspace `.local-test/go-tests.jsonl` (not committed because it is verbose). Browser credentials and the disposable database are not committed.

## Limits and environment notes

- This PR adds PDF annotation UI; non-PDF highlighting is not claimed. New source-bound marks require a non-empty source content hash; old rectangle-only annotations still work. Multi-page text selections are intentionally rejected rather than bound to the wrong page.
- Tests use invented 12/16-page PDF fixtures; chunk/suggestion rows are synthetic reader fixtures, not an ingestion/model-quality claim. No external model call, participant data or production system was used.
- A raw `tsc --noEmit` check encountered existing test-only errors in the quiz and API fixtures; the production Next TypeScript build passed. No unrelated test fixture was changed.
- Local Node 25 exposes an incompatible experimental global localStorage to this jsdom suite. Disabling Node's experimental webstorage made the full suite pass without modifying onboarding code or weakening assertions.
- Existing browser regression needed two harness updates for current main: explicitly continue past optional recall and use the current “About this reading” disclosure rather than its removed tab role. No product navigation behavior was changed.
- GitHub CI is reported blocked by account billing/spending-limit job-start failures. The PR's exact-head check receipt will be recorded separately; local pass does **not** imply remote CI pass. No billing/protection/workflow bypass was made.
- No independent reviewer was spawned (delegation explicitly prohibited nested agents). Maintainer review remains required.
