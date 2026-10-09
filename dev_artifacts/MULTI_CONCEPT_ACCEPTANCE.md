# Multi-concept links — PR #134 integration acceptance

## Dependency and scope

This branch incorporates the complete PR #160 head `7495587f3190590a71db619bf6391a79eb23f650` without rewriting its ancestry. **Merge #160 before #134.** Migration `025_multi_concept_links.sql` follows its annotation migration 024 and format migration 023. The old, unmerged duplicate `021_multi_concept_links.sql` is renamed; the deployed `021_document_practice.sql` and every existing migration checksum are unchanged. Fresh local databases applied 001–025 consecutively and a second runner invocation applied nothing. No production backfill, deployment, repository setting change or merge was performed.

## Contract and concurrency

- Up to five active concept-overlap rows per unordered model pair; already-seen concepts of either direction/status are not suggested again. Legacy reviewed rows are never rewritten/deleted by migration or generation.
- Worker selection and insertion share a transaction-scoped, direction-independent advisory lock. A database insertion trigger uses the same lock and enforces normalized-concept deduplication and the cap for direct writers too.
- Reproduction before the fixes: eight simultaneous opposite-direction workers produced **10** rows; direct persistence produced **14**. Both paths now produce **5** valid, distinct concepts.
- Deterministic subterms are shared term mentions, not entailment, agreement, prerequisite, extension or contradiction assertions. Evidence retains both exact quotes, current document snapshot hashes, chunk hashes, owner/document IDs and locators. Eight DB mutations independently corrupt source/target snapshot, quote, text hash and source identity; all are rejected without creating a row.
- Current reader actions remain flag/retract; legacy confirmed/relabeled storage statuses count toward the cap but are not presented as learner approval.
- The learning-loop browser test now verifies flagging one suggestion removes only that `candidate:<id>` graph edge and preserves the sibling concept cards/edges. The old `mental_link_id` lookup on candidate edges was vacuous and is replaced with actual edge IDs.

## Executed checks

Receipts in `multi-concept-test-logs/` are raw local command outputs:

- Full worker suite with isolated PostgreSQL+pgvector, compiled Go API, built Next console and real Playwright Chromium: **195 passed, zero skipped**. Model calls are fabricated fixtures, not ingestion/model-quality claims. Existing PyMuPDF/FastAPI deprecation warnings remain.
- Full Go suite with `TEST_DATABASE_URL`: every package passed; `go vet ./...` and `go build ./...` passed.
- Console: **62 test files, 415 tests passed** with `NODE_OPTIONS=--no-experimental-webstorage`; production `pnpm build` passed. No console product source was changed by this integration.
- Fresh 001–025 migration receipt; rerun reports `0 adopted, 0 applied, 25 total`.
- Remote CI is recorded separately against the exact pushed head, not inferred from these local results. A successful Claude wrapper can skip review when credentials are absent; it is not an independent review.

The local environment needed Python 3.11 and an isolated scratch TMPDIR (the inherited Python 3.9/lxml setup did not import cleanly). No product dependency was changed to accommodate that local issue.
