# Document format imports: verification and handoff

## Scope

Issues #148 (Markdown/TXT), #149 (native Google Docs), #150 (DOCX), #151 (cross-format gate). Feature PR #157. No PR/issue was merged or closed by this work. Private repository visibility unchanged.

Feature head before this evidence-only stack: `88d3905` incorporates main `76f0dd1` (learning loop and learner docs). Migration `023_document_file_formats.sql` is additive; learning owns 021/022. Annotations must use 024 or later. Both learning practice modules and document_formats remain packaged in Modal. Reader page, library/onboarding, practice modules and learner guides were not edited by the formats implementation; their existing main changes were integrated, not replaced.

## Implemented behavior

- Local and presigned `.md`, `.markdown`, `.txt`, `.docx`, and existing PDF uploads enter the durable ingestion queue. PDF limit remains 50 MiB; other files 10 MiB. Size is checked in planning, multipart, completion and normalization. PDF magic remains required; a ZIP prefix never suffices for DOCX.
- Original filename, original byte SHA-256, import time, stored bytes and immutable snapshot metadata are retained. Presigned staging objects are copied to server-only document keys before queueing, preventing a still-valid PUT from changing reader evidence. Worker hash mismatch fails before embeddings/derived writes.
- Markdown CommonMark parsing retains headings, ordered/unordered list markers, fenced code and tables, with block/line locators. TXT is literal UTF-8. Non-PDF normalization no longer invents lexical joins across hyphen whitespace; PDF provenance regression still passes.
- DOCX validates content-type declaration, internal office-document relationship and WordprocessingML; limits 1,000 entries, 8 MiB per expanded entry, 32 MiB total expanded bytes and 100:1 compression ratio. No archive paths are extracted, macros are rejected and external relationships are never fetched. Simple body paragraphs/headings/tables retain order and part/body locators; table cells render literally. Tracked changes, comments and footnotes/endnotes are explicitly rejected. Pagination, drawings, headers/footers and arbitrary Word layout are not reproduced; full visual fidelity requires a reviewed PDF export.
- Reader suppresses raw HTML, unsafe URL schemes and all remote Markdown images (including tracking pixels). Only stored, authenticated source assets use the existing asset route.
- Native Google Docs use selected-file `drive.file`, native MIME detection without `size`, PDF export and actual bounded response bytes. Metadata records provider file ID/MIME, export MIME/SHA-256, original name, import time and snapshot/no-sync semantics. Bearer tokens are not stored in source metadata, documents or jobs.

## Official Google references checked

Fetched official documentation during implementation (2026-10-09), not live learner data:

- https://developers.google.com/workspace/drive/api/reference/rest/v3/files/export — `GET /drive/v3/files/{fileId}/export`, requested MIME, exported bytes capped by Google at 10 MB, `drive.file` permitted.
- https://developers.google.com/workspace/drive/api/guides/api-specific-auth — selected-file `drive.file` non-sensitive scope, incremental picker access rather than broad Drive access.

Live Google Picker/OAuth/export was **not** exercised. Go HTTP fixtures verify native metadata without size, exported PDF queue/provenance, 401/403/404/503, bad magic, and oversized streaming responses. Valid synthetic PDF ingestion through the real API/worker is separately exercised; do not call that a live Google account check.

## Final local verification after integration

All used an owned disposable local pgvector database `selar_e2e_formats`, TCP **55449**, and synthetic accounts/files only. Shared 55439 and production were untouched.

- `44-integrated-go.txt`: full `go test ./... -count=1` with live isolated DB passed. `go vet ./...` passed.
- `45-integrated-console.txt`: **382 passed**, 57 files. The earlier pre-learning integration total was 385, not the final total.
- `46-integrated-build.txt`: production `pnpm build` passed, including production TypeScript checking.
- `47-integrated-worker.txt`: **167 passed, 7 skipped**, including the cross-format real API → durable queue → Python worker → PostgreSQL test. Skips are explicitly listed: existing Chromium-dependent chat/PDF/web/learning scenarios were not enabled locally in this run.
- `40-eslint.txt`: changed console import/reader files passed ESLint.
- Standalone `tsc --noEmit` earlier reported existing test-only errors in quiz/page.test.tsx and api.test.ts; production build passed. No claim of standalone all-test TypeScript cleanliness.
- Existing dependency warnings remain (PyMuPDF SWIG, Starlette/httpx and Vitest act warnings). Raw logs were retained without rewriting to make them look clean.

## Cross-format provenance acceptance

`test_format_service_e2e.py` starts the real compiled API against isolated PostgreSQL, uploads fabricated Markdown/TXT/DOCX/PDF files and drains real durable worker jobs. Only model/embedding outputs are stubbed. It asserts:

- queued → ready/completed; exact original source SHA-256;
- exact chunk quote within the normalized whitespace witness of its actual content block/locator;
- real database grounded two-sided witness validity, exact snapshot/locator/quote, forged-quote rejection and invalidation after source-hash mutation;
- foreign owner cannot read or delete each source;
- reingestion replaces old chunk IDs while retaining original snapshot hash;
- delete removes source chunks/jobs;
- mutated stored bytes reach failed job state without producing chunks;
- import metadata survives worker normalization.

Provider metadata in the worker service test is explicitly synthetic, inserted to test metadata preservation; it is not evidence of a live provider fetch. Native Docs HTTP import is tested separately in Go as noted above.

The stacked test PR adds this gate to the existing synthetic service workflow while retaining all learning-loop scenarios. CI status must be checked on exact PR heads; no local pass is a claim that GitHub checks or live provider validation ran.

## Logs and integration notes

`formats-test-logs/` contains genuine successive RED/GREEN outputs, intermediate fixture/config failures and final verification. On this macOS host, `umask 022`, scratch `TMPDIR`, and `LC_ALL=C` for PostgreSQL startup were necessary. Worker tests must run from `services/selar-worker` for subprocess module imports. No secrets or real documents are test fixtures.

The independent reviewer step was not run because nested agents were prohibited. A procedural-skill update was attempted but the tool failed with a temporary-directory permission error; it did not change shared agent configuration.

## Independent release review and onboarding integration

A separate reviewer integrated main `8545b8ee46cdb71fb6694780ad372ed798099d0e` into the **top** `test/cross-format-provenance` branch. PR #157 remains pinned at `ddf36c67734ca4306d578e8615274388a0667b94`; no PR was merged or closed.

Concrete findings fixed on PR #158:

- The previously failed `document_formats.py` safety patch was **not present**: the worker used eager `fromstring` without depth/node caps. It now uses incremental defused XML parsing, rejects DTDs and stops above depth 128 or 100,000 nodes per retained XML part, before constructing the entire oversized tree. Existing ZIP byte/ratio/count limits remain enforced. RED/GREEN evidence: 49–52.
- Python `splitlines` treated vertical-tab/Unicode separators as extra Markdown lines, unlike CommonMark, causing a subsequent table to lose its final source row. Source slices now use CR/LF boundaries while preserving original line endings. RED/GREEN evidence: 53–54.
- Newly merged Library import copy and learner docs still said PDF-only/unsupported. Library copy, README and user guide now state supported snapshots and limitations without claiming deployment or live Drive verification. RED copy test: 56; full GREEN: 58.
- PR #158 previously had only the review check because CI/coverage/E2E excluded its stacked base branch. Their branch filters now include `feat/document-format-snapshots`; existing learning-loop scenarios remain in the E2E job.

Audit findings without new blockers: DOCX entries are read through byte limits and CRC validation, never extracted; external relationships are not traversed or fetched. Standard content type and internal office-document relationship are required. Macros, tracked changes, comments and footnotes/endnotes remain rejected; visual/layout omissions are disclosed. UTF-8 upload checks, immutable server-only completion copies, byte-hash validation before model/derived writes, owner-scoped reads/deletion, selected-file Drive authorization, token-free snapshot metadata and Markdown URL/HTML/image restrictions were reviewed with the existing regression fixtures. Synthetic Drive HTTP fixtures are not live OAuth evidence. Arbitrary Word layout fidelity and pixel-identical Markdown are not promised.

Local integrated verification (owned disposable PostgreSQL/pgvector **55459**, database `selar_e2e_review`; shared 55439 untouched):

- 60: all Go tests with isolated DB passed; `go vet ./...` and real API build passed.
- 58: **399 console tests passed**, 59 files. Node 25 required `NODE_OPTIONS=--no-experimental-webstorage`; remote CI retains Node 22.
- 61: console production build and production TypeScript passed.
- 63: **177 worker tests passed, zero skipped**, including the actual API/worker/PG/Next/Chromium service scenarios, formats and learning loop. Model responses are synthetic; no live provider credentials or requests were used.
- Setup failures remain faithfully recorded: 48 used an interpreter missing defusedxml; 55 hit Node 25 localStorage behavior; 57/59 used a nonexistent role in the inherited disposable DB; 62 hit the E2E database-name safety assertion. Corrected runs are separate, not rewritten logs.

Remote exact-head certification belongs in the read-back PR #158 review comment after checks finish; this document does not claim unexecuted checks passed. Parent must merge the complete integrated stack and verify final tree equivalence, not ship immutable PR #157 alone without these top-branch fixes.

