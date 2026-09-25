# Offline graph assertion slice (2026-09-25)

Status: **isolated implementation, not a participant or production feature**. This file reports implementation and synthetic tests, not research efficacy.

## Boundary and authority

- Canonical research Doc ID (provided by the user): `1-MCai2wAhpRfy_R0O0TNggdo5NuYaPrthL-UgR6OlUs`. Per the user's clarification, it calls for **suggesting cross-document links for human confirmation, not automatically mapping them**. The Doc was not edited or independently retrieved for this slice.
- Repository `dev_artifacts/proposal_decoupled_learner_mental_model.md` and `dev_artifacts/KNOWLEDGE_GRAPH_MENTAL_MODEL_REPORT.md` are *architecture proposals*, not authority to alter the canonical intervention, launch a study, or report learning effects.
- `dev_artifacts/ISSUE_5_RESEARCH_PROTOCOL.md` §§28–32 distinguishes cited source chunks from generated answer text and feedback comments. This reducer accepts explicit source document/chunk IDs on **both endpoints**; generated feedback is not treated as source support. The caller must independently validate that those IDs exist, belong to the intended source/owner, and actually support the assertion; this offline model cannot verify that from the database.

## What the standalone module does

`services/selar-api/internal/offlinegraph` is in-memory, with no imports into handlers, stores, worker, or console. It starts from a caller-supplied copy of baseline assertions. Each assertion has a source/library scope (`SourceID`), a directed relation, and separate `FromSupport`/`ToSupport` document + chunk IDs. All identifiers and both support pairs are required. IDs are unique across the ledger's lifetime, even after rollback. No source document, chunk, frozen gold corpus, or initial assertion is rewritten.

`PreviewCorrection` returns a detached before/after proposal and current revision without mutating the ledger. `ConfirmCorrection` requires the exact minted preview plus an explicit matching revision, rechecks active status, support and source scope, then appends a correction action. `Retract` requires a reason and matching revision, and appends a retraction action. `Rollback` requires a reason, a prior target revision, and current revision; it appends a compensating event without truncating history. `SnapshotAt` deterministically reduces initial assertions and actions, including rollbacks, to a historical projection. `Snapshot` returns detached slices. There is no automated promotion, source rewrite, ranking change, reranker, study split edit, participant access, or UI/API/migration integration.

## Observed TDD and local verification

- RED → GREEN: correction test initially failed to compile (`Assertion`, `NewLedger` undefined); then passed.
- RED → GREEN: retraction test failed to compile (`Ledger.Retract` undefined); then passed.
- RED → GREEN: rollback test failed to compile (`Ledger.Rollback`, `Ledger.SnapshotAt` undefined); then passed.
- RED → GREEN: forged preview test failed as expected (`forged preview accepted: <nil>`); then passed after adding a ledger-bound HMAC seal. A modified support ID is likewise rejected.
- `go test ./internal/offlinegraph -count=1 -v`: **7 tests passed**, including invalid/missing support, cross-scope, duplicate or historical IDs, stale revisions, rejected actions, detached snapshots and replay equivalence.
- `go test ./... -count=1`: **passed** (Go API packages); `go test -race ./internal/offlinegraph -count=1`: **passed**; `go vet ./...`: **passed**; `go build ./...`: **passed**; `gofmt -l internal/offlinegraph/*.go`: **clean**; `git diff --check`: **clean**.
- PR #27's first remote check run for commit `ae6141f` passed all eight checks: Build Go API (including newly added `go test ./... -count=1`), Build Next.js Console, Check Python Worker, Go API Coverage, Console Coverage, Worker Coverage, Dependency Review, and Claude / PR Review. This is automated CI, not independent human review. The documentation update itself requires a fresh check run before merge.
- CI workflow now executes `go test ./... -count=1` in addition to its existing build/vet checks.

## Limitations / deliberate non-goals

This is a synchronous, single-owner in-memory reducer, not persistent storage, concurrent transaction handling, an authenticity check against source text, or a production authorization boundary. The HMAC seal prevents accidental/fabricated preview structs inside the offline process; it is not a network security protocol or durable token. The source scope is supplied by the caller rather than resolved to a DB owner. Retraction changes the offline active projection, never the cited chunk. Rollback restores an earlier *projection*, retaining every historical event and never reusing prior assertion IDs. No frozen v0.5 gold data/splits were accessed or changed; no participant pathway is connected. No efficacy or recall outcomes are inferred from synthetic tests.
