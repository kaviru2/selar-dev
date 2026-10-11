# Local reading-practice review

Status: local candidate only. Do not push or deploy without approval for the final local head.

## Scope and tracking

- Isolated clone: `/Users/kavirum1a/.hermes/cache/scratch/selar-reading-humanized`.
- Branch: `local/reading-humanized`; baseline: `1ac70e2fa24e4daf52c441fbdc808a7ae6ddfa00`.
- Read-only issue inspection found #14 (participant-study readiness blocker) and #9 (provenance-aware chat/graph). This change does not resolve #14 or change the formal study protocol. No issue/PR was written remotely.
- No push, merge, deployment, external database write, participant access or messages. Existing SQL migrations were applied only to a newly initialized scratch PostgreSQL database bound to loopback on port 55479. The server is stopped after verification.

## Changes

- Before reading now offers optional, ephemeral prior-knowledge notes and an explicit “I don't know yet” route. It does not show the later retrieval questions, reference answers, grade notes, or submit a warm-up attempt. Legacy warm-up RPC compatibility is unchanged; formal quiz/cohort/reading-anchor code is untouched.
- Generation instructions prioritize central ideas, mechanisms, meaningful contrasts and limitations. They permit meaningful factual recall, ask for one natural target, and allow no question instead of padding. Surface rejection gates catch common citation/list trivia, vague final referents, multiple question marks and complete lexical answer leakage. Exact quote, chunk hash and locator checks remain.
- One rejected passage no longer discards supported practice from another passage. The existing whole-request timeout still publishes nothing.
- Exact-restatement grading now requires matching content-word sequences rather than answer-word inclusion: extra claims go to semantic grading. Grading instructions allow paraphrases and request specific, plain feedback. Uncertain feedback remains unscored, and reference answers/quotes appear only after retrieval submission.
- `practice-meaningful-v3` filters retired items out of every shared LIVE read (items, due queue, attempts and progress). The existing unique source/chunk key is unchanged. Regeneration can replace only unattempted, unscheduled retired payloads. Attempted history is never overwritten or deleted; this can leave fewer/no current questions.
- Invalidated pending cache reads cannot repopulate the cache or remove newer in-flight reads.

## Verified tests

Baseline relevant worker tests: 21 passed, 8 database skips before creating the isolated test database. Initial default console run on Node 25.5.0: 489 passed, 17 failures in StartHereGuide/library-onboarding due to native webstorage interfering with jsdom. With `NODE_OPTIONS=--no-experimental-webstorage`, an untouched baseline worktree passed all 506 console tests; full baseline worker suite passed 229 with 9 dedicated service/browser skips.

Observed RED → GREEN slices:

- Supplied trivia/list patterns: 3 failures before production gates, then accepted mechanisms/meaningful facts plus rejected-pattern fixtures passed.
- Before-reading leakage: new DOM interaction test failed because the later question was visible; passed after separating prior notes.
- Retired version: 2 real PostgreSQL regressions failed because old questions were still returned; passed after version-aware reads and guarded regeneration.
- Partial generation: regression failed because one unsuitable passage discarded the good item; passed after filtering rejected drafts.
- Extra-claim grading: regression caught incorrect deterministic full credit; passed after stricter restatement matching.
- In-flight cache invalidation: regression caught stale cache restoration; passed after request-identity checks.
- Unscored feedback: DOM submission test failed on the old heading, then passed with “AI feedback · not scored”, while checking source disclosure and reading_check/exposed request fields.

Final commands (from the respective service directories):

```sh
NODE_OPTIONS=--no-experimental-webstorage pnpm test
TEST_DATABASE_URL='postgres://selar_local@127.0.0.1:55479/selar_local?sslmode=disable' .venv/bin/python -m pytest -q -rs
pnpm exec eslint components/ReadingPractice.tsx components/ReadingPractice.prior-knowledge.test.tsx lib/practice-cache.ts lib/practice-cache.test.ts
git diff --check
```

Console: 509 passed across 74 files. Worker: 243 passed, 9 dedicated service/browser skips, 15 warnings (dependency deprecations and unregistered timeout marks). Console baseline/final and worker baseline/final logs are in `.local/` (locally excluded from git alongside scratch database files). Changed-file ESLint and whitespace check passed.

`pnpm exec tsc --noEmit` reports two errors, independently reproduced at the untouched baseline: `app/(app)/quiz/page.test.tsx` TS2786 and `lib/api.test.ts` TS2741. No practice-code type errors were reported. This is not a clean repository-wide typecheck/build claim.

## Sample wording

Before reading: “What, if anything, do you already know about this reading's topic?” with “I don't know yet” and an unscored-notes disclosure.

Hand-authored synthetic fixtures, not live-model or actual-document outputs:

- “How does a retry recover from a temporary failure?” → “repeats the failed operation”; exact fixture quote: “A retry repeats the failed operation after a temporary failure.”
- “What prevents a retry from creating duplicate payments?” → “An idempotency key”; exact fixture quote: “An idempotency key prevents a retry from creating duplicate payments.”
- “Who decides whether to retry a failed operation?” → “The coordinator”; exact fixture quote: “The coordinator decides whether to retry a failed operation.”

Exact-restatement feedback: “Your answer matches the reference wording. Compare it with the source below.”

## Not proven

- No paid/live provider call was made. Surface filters and lexical support are conservative gates, not proof of semantic question/answer correctness, model adherence, or educational benefit. Paraphrases still need provider grading; a provider outage leaves those attempts unscored.
- The actual user document was not available to the unauthenticated fetch: curl followed the supplied reader URL to `/login?from=%2Freader` with HTTP 200. No login was attempted and no passage content was fabricated.
- Full API/Next/Chromium service E2E is not executed here; 9 dedicated scenarios remain skipped. The synthetic learning-loop harness was updated for unscored preparation, but its browser path is not claimed verified.
- No production cache migration, deployment rehearsal, participant study, mastery or delayed-retention benefit is established.
- Existing source selection still examines at most three chunks, ordered by chunk ID. Document-wide coverage is not established.
- Without changing the schema, attempted retired rows may block replacement for the same source/chunk. Keeping history takes precedence over silently rewriting past practice. Legacy warm-up attempts remain stored; new UI preparation does not create them.
- Parent review is still needed before presenting this candidate for push/deploy approval. No independent reviewer subagent was available in this focused runtime.
