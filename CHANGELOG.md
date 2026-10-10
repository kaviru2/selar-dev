# Changelog

All notable changes to SELAR are recorded here. SELAR is a research prototype (University of Colombo School of Computing). Versions follow [Semantic Versioning](https://semver.org/). Numbers in brackets are pull requests.

AI suggests, sources show, learner explains.

## [0.2.0] - 2026-10-10

The first release since v0.1.0, covering 123 merged pull requests. It adds the learning loop, a rebuilt Reader, accounts and settings, quizzes run by admins, more import formats, and the production deployment and operations tooling.

### Learning loop
- Private practice questions grounded in the learner's own documents [#141]. Generated practice is practice only.
- Warm-up before reading and an end-of-reading check in the Reader [#142].
- Spaced review with a daily session and a UTC-day streak [#143].
- Neutral reflection links with "Compare passages" and a "This link is wrong" flag. There is no confirm step [#144, #96].
- Progress page with consent-gated practice analytics [#145].
- Integrated real-service end-to-end tests and safety gates for the whole loop [#152].
- Lightweight first-reading guide and a single library import entry point [#154].

### Reader
- Continuous scroll with virtualised pages, find in document, thumbnails and a remembered position [#104].
- Highlights and notes tied to the source text, with a highlights list [#160, #22].
- Collapsible, resizable Library and Connections panels [#102].
- Restyled reflection prompts, practice and graph states on shared design tokens [#162].
- Real reader interaction telemetry [#18].

### Links, graph and chat
- Runtime mental-model graph and evidence-grounded chat, with graph governance [#2, #4, #6].
- Link candidates grounded in both source passages, and generated in both directions regardless of ingestion order [#30, #121, #38].
- Up to 5 grounded concept links per pair of readings [#134].
- Candidate links awaiting review shown on the Graph, with clear empty states [#123, #124].
- Research assertions that record their provenance. Chat can propose a graph update for the learner to review [#120, #122, #27, #34].
- Owner-selected saved assertions shown safely in chat [#161].
- One answer-feedback model: 👍/👎, an optional reason, and "Report a wrong claim" [#165]. Unhelpful feedback withdraws that answer's evidence from the graph [#115, #13].
- Chat fails closed: explicit graph commands are refused, and so is attribution to an original source without proof [#63, #48, #44].

### Sources and import
- Durable, recoverable ingestion with multimodal (figure-aware) PDF processing [#8, #7].
- Snapshot imports from Markdown, TXT, DOCX and Google Docs, with size limits [#157, #158].
- Import from Google Drive (Picker, `drive.file` scope only) and an accurate Drive status in the sidebar [#129].
- Safer web ingestion: DNS-rebinding protection, IPv6 host handling, and hyphens at PDF line ends kept in source text [#56, #57, #67].

### Accounts, settings and privacy
- Redesigned sign-in and registration. Registration is protected by reCAPTCHA Enterprise, and Sign in with Google is available [#99, #111, #118].
- Settings work end to end: account, email, password, appearance, reader defaults, suggestions, privacy and data [#112, #94, #103].
- Export my data and delete my account. Deleting an account keeps only an anonymous withdrawal record [#100, #131].
- Optional, consent-gated research-usage analytics [#97, #106].
- The study group's suggestion setting is enforced on the server and shown as a locked toggle [#132, #133].

### Quizzes (admin-authored)
- Quiz system with time windows, attempts and import enforced on the server [#92].
- Learner take-flow and admin editor, results and blind grading [#105, #107].
- Optional calendar feed for quiz windows, off by default [#119].
- Dry-run-by-default study email notices: opt-in, with unsubscribe and an audit trail [#127]. Email and reminders stay off in production.

### Design
- SELAR brand identity, public landing page, dark theme, and a redesigned app shell and library [#98, #83, #95, #126].

### Performance
- PDFs reopen from a per-user cache in the browser instead of being downloaded again. The first page renders from HTTP range requests. In a local measurement with a 19.5 MB PDF, a reopen went from 6.3–6.7 s to under 1 s, with no download [#168].
- Practice reads are served directly by the API, and Review and Progress reopen instantly [#166].
- The worker uses gemini-2.5-flash-lite with thinking off and fewer model calls for practice [#167].

### Operations
- One-command production deploy with migrations and a 15-point smoke test [#93].
- Nightly encrypted Postgres backups to GCS, with a tested restore [#116, #125, #130].
- Production hardening: development JWT secrets are rejected, the worker stays on the internal network, and registration email is validated [#24, #20, #26].
- Fewer avoidable GitHub Actions runs, with the same checks kept [#156].

### Documentation
- Concise README with illustrations, a verified local quickstart, and a refreshed GitHub Pages site [#153, #163, #164].
- Research traceability records for study safeguards and acceptance evidence [#27–#68].

## [0.1.0] - 2026-04-27

- Initial prototype:
  - Go API with pgvector schema and authentication.
  - Next.js console with library, PDF reader and annotations.
  - Worker that extracts links and the knowledge graph with Gemini.

[0.2.0]: https://github.com/kaviru2/selar-dev/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/kaviru2/selar-dev/releases/tag/v0.1.0
