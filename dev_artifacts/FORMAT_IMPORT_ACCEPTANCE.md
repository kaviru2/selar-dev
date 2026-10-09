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
