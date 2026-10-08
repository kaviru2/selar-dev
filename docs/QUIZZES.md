# Quizzes

SELAR quizzes cover the study's **initial test**, **follow-up tests** (for example the day-7 delayed recall test) and optional **practice** quizzes. Admins write a quiz once as a file or in the editor, publish it, and the API enforces who can take it, when, how often, for how long, and when answers are revealed.

Tracking issue: #86.

> **Research integrity.** This repository contains only invented DEMO quiz content (the "Zorb fruit" templates). The study's real initial and day-7 items, answer keys and rubrics need **supervisor and ethics sign-off before anyone enters them**, and real participant data must not be collected until ethics approval is in place (issue #14). Treat the quiz system as infrastructure, not as an approved instrument.

## Deploy a quiz in under two minutes

1. Download a template from **Admin → Quizzes → Import** (`/quiz-templates/quiz-template.yaml`, `.md` or `.json`).
2. Edit the title, settings and questions.
3. On **Import**, drop the file in (or paste it). The preview shows every question and any validation error with its line or question number.
4. Click **Import as draft** (or **Import and publish**).
5. Open the quiz, check the learner preview, and click **Publish**. Learners see it at `/quizzes` once it opens for them.

The same works from a terminal with an admin JWT:

```bash
curl -X POST "$API/api/admin/quizzes/import?filename=quiz.md&publish=1" \
  -H "Authorization: Bearer $TOKEN" --data-binary @quiz.md
```

`?dry_run=1` validates and returns the parsed quiz without saving it.

## Who is an admin

Admin routes (`/api/admin/...`, console `/admin/quizzes`) require an admin. Until the shared `users.role` admin check lands, the API reads a comma-separated **`ADMIN_EMAILS`** environment variable. If it is empty, nobody is an admin. The check is behind the `AdminChecker` interface so it can be swapped for the role-based check without changing quiz code.

## Concepts

| Setting | Values | Notes |
|---|---|---|
| `kind` | `initial`, `follow_up`, `practice` | Labels the quiz and sets the feedback default. |
| `status` | `draft`, `published`, `closed` | Drafts are invisible to learners. Closing ends the quiz for everyone and auto-submits open attempts. |
| `open_at`, `close_at` | RFC 3339 with a zone, e.g. `2026-10-10T09:00:00+05:30` | Absolute window. A time without a zone is rejected. |
| `after` | `{anchor, days, window_days}` | Per-learner relative window, see below. Intersected with `open_at`/`close_at`. |
| `time_limit_minutes` | number | Server deadline per attempt. The deadline is the earlier of start + limit and the learner's close time. |
| `max_attempts` | integer, `0` = unlimited | Counted per learner. Default `1`. |
| `feedback` | `never`, `after_submit`, `after_close` | When scores, correct answers, explanations and sources are shown. Default `never` for `initial`/`follow_up`, `after_submit` for `practice`. |
| `audience` | `all`, `cohort:<cohort>`, `group:<label>` | Cohorts are `users.cohort`. Groups are lists of emails managed under **Admin → Quizzes → Groups**. |
| `shuffle_questions`, `shuffle_options` | bool | Fixed per attempt (seeded by the attempt id), so a reload shows the same order. True/false is never shuffled. |
| `no_going_back` | bool | One question at a time; the API only serves the current question and rejects answers to earlier ones. Intended for recall tests. |

### Relative availability

| `anchor` | Opens `days` after… | Extra field |
|---|---|---|
| `first_reading` | the learner's first reading session in SELAR | — |
| `document` | the learner's first reading session on a document | `document`: the document's content hash or exact title |
| `quiz_submitted` | the learner first submitted another quiz | `quiz_id`: that quiz's id |

`window_days` (optional) closes the quiz that many days after it opens. Before the anchor happens, the quiz shows as **Upcoming** with the reason ("Opens after your first reading session") and no date.

A typical delayed test: `after: {anchor: quiz_submitted, quiz_id: <initial quiz id>, days: 7, window_days: 2}`.

### Question types and grading

| Type | Answer | Grading |
|---|---|---|
| `single_choice` | one option | Automatic. |
| `multiple_choice` | any options | Automatic, all-or-nothing (the exact set of correct options). |
| `true_false` | True / False | Automatic. |
| `short_answer` | text | Automatic when it matches an `answers` entry after lowercasing, collapsing spaces and removing trailing punctuation; otherwise queued for manual review. |
| `cued_recall` | text, shown with a `cue` | Always manual, unless blank (scored 0). |
| `free_recall` | long text | Always manual, unless blank (scored 0). |

Each question has `points` (default 1), and optional `rubric` (admin-only, shown to graders), `explanation` and `source` (`{document, url}`), which are shown to the learner only when feedback is allowed. An attempt's final score exists only once every manual item is graded; until then results show "pending review".

Manual grading (**Admin → Quizzes → Results → Grade**) shows answers without the learner's email or cohort so it can be done blind.

## File formats

### YAML / JSON

JSON files use the same keys (JSON is valid YAML). Unknown keys are rejected so typos do not silently change a quiz.

```yaml
title: "DEMO: Zorb fruit practice quiz"
description: Invented demo content.
kind: practice
feedback: after_submit
max_attempts: 1
time_limit_minutes: 10
audience: all
shuffle_options: true
questions:
  - type: single_choice
    prompt: What colour is a ripe Zorb?
    options: ["Red", "*Blue", "Green"]   # * marks the correct option
  - type: multiple_choice
    prompt: Pick the invented moons.
    options:
      - {text: Ith, correct: true}
      - {text: Ombra, correct: true}
      - {text: Luna}
  - type: true_false
    prompt: Zorbs grow underwater.
    answer: false
  - type: short_answer
    prompt: Name the valley.
    answers: [Vell, Vell Valley]
  - type: cued_recall
    prompt: Complete the rule.
    cue: "Zorbs ripen when ..."
    rubric: Credit any mention of the second moon.
    source: {document: "DEMO reading", url: "https://example.com/demo"}
  - type: free_recall
    prompt: Write everything you remember about Zorbs.
    points: 5
```

### Markdown

Optional YAML front matter holds the settings. `#` is the title, text before the first question is the description, and each `## <type> (N points)` starts a question. The prompt is the text under the heading.

```markdown
---
kind: follow_up
no_going_back: true
---
# DEMO: Markdown quiz

Description text.

## single_choice (2 points)
Which invented river feeds Vell?
- [ ] The Orn
- [x] The Sable

Explanation: The demo reading names the Sable.

## true_false
Zorbs are blue.
- [x] True
- [ ] False

## short_answer
Name the valley.
Answer: Vell
Answer: Vell Valley

## cued_recall
Recall the ripening rule.
Cue: Zorbs ripen when ...
Rubric: Credit any mention of the second moon.
Source: DEMO reading

## free_recall (5 points)
Write everything you remember.
```

Any quiz can be exported back to YAML from its admin page, so quizzes can be versioned in git or copied between environments.

## Integrity guarantees (server-side)

- Drafts, out-of-audience quizzes and quizzes outside the learner's window cannot be started (`409`).
- One open attempt per learner and quiz (unique index); attempt counts are enforced on start.
- Deadlines are computed on the server at start. Saving after the deadline or after the quiz closes auto-submits the attempt (`submit_reason` = `time_limit` / `closed`) and the save is rejected.
- Learner attempt payloads never contain correct flags, accepted answers, rubrics, explanations or source links. Results include them only when the feedback policy allows; with `never`, the learner only sees that the attempt was submitted.
- Once an attempt is submitted its answers cannot change (API check plus a database trigger); only grading columns can.
- Questions are frozen once any attempt exists, so every learner answers the same items with the same key. Settings (window, audience, attempts, feedback) can still change. To change items, duplicate the quiz.
- Time on question is reported by the client and capped by the wall-clock time since the attempt started.

## Data captured

Per attempt: start, server deadline, submit time and reason, quiz version, question and option order shown, score totals. Per answer: selected option ids or text, first seen, last answered, number of revisions, time on question, automatic score and correctness, manual score, grader note, grader and time.

**Exports** (Admin → Results): `results.csv?level=attempt` (one row per attempt) and `?level=answer` (one row per attempt × question). Rows carry the pseudonymous user id and cohort, never the email. Text answers that start with `=`, `+`, `-` or `@` are prefixed with `'` so spreadsheets do not run them as formulas.

## Protocol-relevant choices (for supervisor review)

These are defaults in the software, not study decisions. Each needs confirming against the approved protocol.

1. **Feedback defaults to `never` for `initial` and `follow_up` quizzes.** Showing answers after the initial test is itself a learning event and could inflate day-7 recall in both arms.
2. **Quiz pages show no reader content, links or chat.** Follow-up tests do not show the learner's earlier SELAR links or notes. If the protocol wants cued recall with the learner's own links, that must be added deliberately as a cue.
3. **Multiple choice is all-or-nothing.** Partial credit would change score distributions.
4. **Short-answer matching is normalised exact match only.** Anything else goes to a human, so automated scoring never guesses.
5. **Manual grading is blind to email and cohort** in the console. The CSV export includes cohort for analysis.
6. **Relative windows anchor on events in SELAR** (first reading session, a document, or quiz submission). A "day 7" test anchored on `quiz_submitted` measures 7 × 24 h from submission, not calendar days in the learner's time zone.
7. **No-going-back is optional per quiz.** It prevents revisiting earlier recall prompts once later prompts may have cued them.
8. **Focus/visibility changes are not recorded.** The system does not proctor; it logs timing only.
9. **Practice quizzes are visible to everyone by default.** For the study, restrict any practice quiz to an audience, or keep it unpublished, so it does not become an extra treatment.

## API reference

Learner (authenticated):

| Method | Path | |
|---|---|---|
| GET | `/api/quizzes` | Cards with availability: `available`, `in_progress`, `upcoming`, `completed`, `missed`. |
| POST | `/api/quizzes/{id}/attempts` | Start or resume an attempt. |
| GET | `/api/quiz-attempts/{id}` | Current attempt view (no keys). |
| PUT | `/api/quiz-attempts/{id}/answers` | Autosave `{question_id, selected[], text, time_on_question_ms}`. |
| POST | `/api/quiz-attempts/{id}/advance` | No-going-back: move to the next question. |
| POST | `/api/quiz-attempts/{id}/submit` | Submit (idempotent); returns the result. |
| GET | `/api/quiz-attempts/{id}/result` | Result filtered by the feedback policy. |
| GET | `/api/admin/access` | `{admin: bool}` for the current user. |

Admin: `GET/POST /api/admin/quizzes`, `POST /api/admin/quizzes/import`, `GET/PUT/DELETE /api/admin/quizzes/{id}`, `POST .../{id}/status`, `POST .../{id}/duplicate`, `GET .../{id}/export`, `GET .../{id}/attempts`, `GET .../{id}/stats`, `GET .../{id}/results.csv`, `GET /api/admin/quiz-attempts/{id}`, `PUT /api/admin/quiz-answers/{id}/grade`, `GET /api/admin/quiz-groups`, `PUT /api/admin/quiz-groups/{label}`.

## Schema

Migration `012_quiz_system.sql` replaces the unused placeholder tables from `001` (it refuses to run if they contain rows) with `quizzes`, `quiz_questions`, `quiz_group_members`, `quiz_attempts` and `quiz_answers`.
