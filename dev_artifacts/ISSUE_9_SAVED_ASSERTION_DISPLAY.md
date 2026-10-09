# Issue #9 — explicit saved-assertion display

This is a bounded read-only display mode, not a solution to arbitrary research QA and not closure of issue #9.

## Contract

The owner explicitly picks an asserting document and an existing confirmed assertion in Chat. The picker constructs a canonical JSON tuple of subject, predicate, object, scope, experiment context, subject qualifier and object qualifier. The API carries the selected document ID, assertion ID and revision to the worker alongside the authenticated owner ID.

The worker accepts only that exact tuple. It does not perform semantic matching, alias expansion, extraction or LLM entailment inference. A changed predicate, subject, object, qualifier, scope or context, an appended factual question, and a free-text original-source question all abstain in this mode. In particular, TAU Benchmark is not substituted for τ²-bench, and modified implementations are not treated as originals.

One SQL statement checks owner, document, assertion, revision, confirmed/non-superseded state, document readiness and every remaining witness's document/owner, full text hash and exact quote membership. Missing or stale selections produce a deterministic abstention with no citations. The picker is only a convenience; authorization and freshness checks are performed again by the worker.

The answer displays the saved tuple as escaped fenced JSON, with selected-document citations and explicit limitations: an owner-confirmed saved claim is not independently verified truth, quote membership is not entailment, and selecting a document does not establish original-source identity. Original-source factual QA outside this mode retains its existing conservative path.

No model calls or graph/learner updates are performed for this display. Helpful, unhelpful and correction feedback remain recordable but do not adapt graph or learner projections for this response type. Assertion proposal/confirmation remains a separate workflow.

## Verification — October 9, 2026

Resumed an interrupted dirty tree without discarding its implementation. The inherited model, worker and picker tests initially passed; this is not a claim that this resumed run observed their original RED phase. The packaging test genuinely failed because Modal omitted `assertion_chat`; adding that module made it pass. Added API forwarding/validation and full service/browser regression coverage.

Isolated native PostgreSQL/pgvector cluster: loopback port **56449**, databases `assertion_tests` and `selar_e2e_assertions`, all 25 migrations applied. No shared database (including 55439), participant data, frozen study material, production deployment or live model provider was used.

Executed results:

- `TEST_DATABASE_URL=postgres://selar@127.0.0.1:56449/assertion_tests?sslmode=disable go test -count=1 ./...`: all packages passed, including database-backed owner, feedback and governance tests.
- `go vet ./...` and API server build: passed.
- `NODE_OPTIONS=--no-experimental-webstorage npm test`: **448 passed / 66 files**. Native Node 25 initially caused 17 existing browser-storage test failures; disabling Node's experimental server-side Web Storage restored the intended jsdom storage implementation without repository changes.
- `npm run build`: production console compilation and TypeScript checking passed.
- Full worker suite with `SELAR_SYNTHETIC_E2E=1`, the dedicated DB, built API and console, Playwright Chromium, empty model keys and queue polling disabled: **209 passed, no skipped tests**. Existing PyMuPDF/SWIG deprecation warnings remain.
- The real browser scenario selects the document/assertion, verifies the exact composer tuple, sends through the Next cookie proxy → Go → worker HTTP → SQL, and observes the stored display answer. It also checks proposed, stale-revision, foreign-owner/document, changed tuple, changed witness hash, processing document, retracted and superseded abstentions; graph equality after all three ratings; and no chat graph-update rows.

The service test is imported into `test_chat_service_e2e.py`, which the existing synthetic CI job already runs; no workflow changes are needed. Modal packaging coverage checks the new runtime module.

Raw local receipts are retained under `.local/receipts/` in the implementation workspace (not committed): `go-final.txt`, `console-final.txt`, `console-build.txt`, `worker-final-all-e2e.txt` and `assertion-service.txt`. The latter records the focused four-scenario service run before consolidation into the existing CI test entrypoint. Remote CI is tracked on the PR, not asserted by this local receipt.

## Remaining boundaries

- No claim that an exact stored quote actually entails its assertion.
- No automatic primary-document identification or general factual-answer support.
- Historical displays are snapshots, not a live assertion-status view after later retraction.
- No change to formal-study readiness, consent/ethics locks, or efficacy claims.
