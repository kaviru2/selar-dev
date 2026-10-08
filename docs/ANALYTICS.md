# SELAR analytics and admin access

SELAR has a small first-party usage-analytics system and an admin area (`/admin`).
This page covers the event dictionary, the privacy rules, how other features record events,
and how someone becomes an admin. Tracking issue: #84.

> **Research-integrity note.** This is prototype instrumentation, not an ethics-approved
> data-collection instrument. Do not use SELAR with real study participants until ethics
> approval is in place and issue #14 is closed. The in-app consent checkbox is product consent
> for usage analytics only. It does not replace the participant information sheet and the
> consent form required by the ethics protocol.

## What is collected, and where

- **Storage.** Events are stored only in SELAR's own Postgres database (`analytics_events`,
  plus `analytics_daily_counts` for anonymous totals). No third-party tracker, script, pixel or
  analytics service is used, and the system adds no cookies or browser identifiers. The console
  sends events to `/api/analytics/events` on its own origin, and the existing httpOnly session
  identifies the user.
- **Server-side truth first.** Anything the API can observe itself (uploads, decisions, chat
  questions, quiz scores, reading sessions) is recorded by the Go API. The browser cannot submit
  these events: the API rejects server events sent from a client. The console only reports
  actions that happen purely in the UI (page views, dwell time, a suggestion being shown, the
  explain step and the compare step).
- **No free text.** Every event has a fixed list of properties. Each one is type-checked as a
  UUID, an enum, a bounded integer, a boolean or a same-origin path. Anything else is dropped on
  the server. Chat questions, explanations, recall text, notes, passages and emails can never be
  stored through this path. For text the user writes, only its **length** is recorded. A future
  feature may store text only with a separate, explicit opt-in and a new consent version.
- **Consent gates per-user data.** A per-user event row is written only for users who opted in
  (`users.consented_at IS NOT NULL`). The check happens inside the same SQL statement as the
  insert, so a withdrawal that commits first always wins. Separately, every accepted event
  (from any user, consenting or not) increments an anonymous per-day count per event name
  (`analytics_daily_counts`, which has no user id). That count is all a non-consenting user ever
  contributes.
- **Kill switch.** Set `ANALYTICS_ENABLED=false` on the API to stop all recording, including
  anonymous counts. The default is enabled, but per-user rows still require consent.

### Consent

- **Registration** shows an optional, unticked checkbox: "Help improve SELAR: record how I
  use the app (no text I write is recorded)". The account works the same either way.
- **Users who registered before this feature,** or who skipped the checkbox, see a one-time
  prompt after login with the same wording and "Yes" / "No thanks" buttons. Either answer sets
  `consent_decided_at`, so the prompt does not return.
- **Settings → Privacy & data** shows the current state. From there the user can turn analytics on or
  off and delete their analytics data.
- The user record stores `consented_at`, `consent_version` (currently `prototype-usage-v1`, see
  `analytics.ConsentVersion`) and `consent_decided_at`. Bump the version whenever the wording
  changes.

### Retention and deletion

- `DELETE /api/users/me/analytics` (**Settings → Privacy & data → Delete my usage analytics**)
  deletes all of the user's events immediately. Turning analytics off stops new events but
  keeps old ones until the user deletes them.
- Deleting a user account deletes all of that user's events (`ON DELETE CASCADE`).
- No automatic retention window is enforced yet. **Protocol decision needed:** the ethics/DMP
  retention period should be set. A scheduled purge can then be added, for example
  `DELETE FROM analytics_events WHERE occurred_at < now() - interval 'N days'`.

### Exports

**Admin → Export** downloads CSV files of events or of the per-user summary for the selected
dates and group. Exports contain the user UUID and the group label but **no email addresses**,
unless the admin ticks "Include email addresses" (`?include_email=true`). Password hashes are
never selected by any analytics or admin query. Text cells are escaped against spreadsheet
formula injection.

## Event dictionary

Times are UTC. "Source" says who may emit the event. The authoritative copy is the Go map in
`services/selar-api/internal/analytics/analytics.go`; `GET /api/admin/events` returns it as JSON.
A test fails if an event is missing from this table.

| Event | Source | When | Properties |
|---|---|---|---|
| `signed_in` | server | Login or registration succeeded | `method` (login, register) |
| `research_consent_granted` | server | User opted in | `via` (register, prompt, settings) |
| `analytics_data_deleted` | server | User deleted their analytics (anonymous count only, never per user) | — |
| `document_uploaded` | server | A document was accepted and queued | `document_id`, `source_type` (pdf, web, text) |
| `reading_session_started` | server | Reader opened a document | `document_id`, `session_id` |
| `reading_session_ended` | server | Reading session closed (duration computed by the server) | `document_id`, `session_id`, `duration_ms`, `pages_viewed` (count), `max_scroll_depth` (0–100) |
| `highlight_created` | server | Highlight, underline or note saved (note text not recorded) | `document_id`, `page`, `type` |
| `decision_made` | server | Keep / change / reject a suggestion, or retract / undo | `kind` (mental_link, passage_link, concept, concept_edge), `action` (keep, change, reject, retract, undo), `link_id`, `response_ms` |
| `chat_question_asked` | server | Grounded-chat question sent | `thread_id`, `length` (characters) |
| `chat_citation_opened` | server | A chat citation was opened | `citation_id` |
| `quiz_started` | server | Quiz attempt started (emitted by the quiz handlers) | `quiz_id`, `attempt_id`, `phase` (quiz kind: initial, follow_up, practice), `question_count` |
| `quiz_submitted` | server | Quiz attempt submitted and scored | `quiz_id`, `attempt_id`, `phase`, `question_count`, `score_pct` (0–100), `duration_ms` |
| `app_session_started` | client | Console opened in a tab | `route` |
| `app_session_ended` | client | Tab hidden or closed (visible time only) | `duration_ms` |
| `page_viewed` | client | Console route shown (path only, no query) | `route` |
| `reader_page_viewed` | client | Reader left a page: visible dwell, ≥1 s, capped at 30 min | `document_id`, `page`, `dwell_ms` |
| `suggestion_shown` | client | A suggested connection became visible | `kind`, `link_id`, `position` |
| `suggestion_opened` | client | User moved from "notice" to "explain" | `kind`, `link_id` |
| `explain_submitted` | client | User left the explain step (lengths only, never text) | `link_id`, `length`, `recall_length`, `recall_on` |
| `compare_viewed` | client | Source passages shown side by side | `kind`, `link_id` |

### Dashboard definitions

- **Active users.** Distinct consenting users with at least one event in the window. "1 d" and
  "7 d" count back from the end of the window.
- **Suggestions shown vs decided.** `suggestion_shown` events against `decision_made` events.
  Keep, change and reject rates are shares of all decisions.
- **Reading time.** Sum of `reader_page_viewed.dwell_ms`, which counts visible time only and caps
  each page at 30 minutes. This is a **usage proxy**, not a measure of attention or learning.
- **Quiz score.** Mean `score_pct` of `quiz_submitted` events (score / max points from the
  attempt row, computed by the server; attempts awaiting manual grading have no score yet).

These are interaction proxies. They do not measure learning, retention or comprehension.

## Recording events from code

### Go (API, server-side truth)

```go
import "github.com/selar-dev/selar-api/internal/analytics"

h.Track(r.Context(), userID, analytics.QuizSubmitted, analytics.Props{
    "quiz_id": quizID, "attempt_id": attemptID, "phase": "follow_up",
    "question_count": 10, "score_pct": 80, "duration_ms": elapsed.Milliseconds(),
})
```

- `Track` never fails the caller's request. It is a no-op when analytics is disabled or the user
  has not consented (anonymous count only), and it only logs errors. It writes synchronously with
  a 2-second deadline, because serverless functions freeze after responding.
- Call it **after** the action succeeded, so failed requests are not counted.
- Adding an event means adding it to the dictionary in `internal/analytics/analytics.go` with
  typed properties, adding a row to the table above, and setting its `Source`. Free-text
  properties are not allowed.

### TypeScript (console, UI-only actions)

```ts
import { track } from "@/lib/analytics";

track("compare_viewed", { kind: "mental_link", link_id: link.id });
```

- `track()` is typed against `ClientEventProps` and is a no-op until `AnalyticsProvider` enables
  it for a consenting user. Callers never check consent themselves.
- Events are queued and sent in batches of up to 50, every 10 seconds, and with `sendBeacon`
  when the tab is hidden.
- For per-page reading time, use `createDwellTracker` from the same module instead of sending
  an event on every scroll.

## Roles and admin access

`users.role` is `user` (default) or `admin`. The API reads the role from the database on every
admin request; it is not stored in the JWT, so promotion and demotion take effect immediately.
Admins sign in through the normal login page.

- **API.** Everything under `/api/admin/*` passes through `auth.Verify` and then
  `middleware.RequireAdmin`. Anonymous requests get 401, signed-in non-admins get 403, and a
  failed role lookup is treated as a denial.
- **Quizzes.** `/api/admin/quizzes*` uses the same `users.role` check (`h.RoleAdminChecker()`).
- **Console.** `/admin` checks the role on the server (`requireAdmin()` in the admin layout
  calls `forbidden()`), so the page is protected and not merely hidden. The **Admin** nav item
  only appears for admins.

### Making someone an admin

1. **`ADMIN_EMAILS`** (bootstrap): set a comma-separated list on the API project, for example
   `printf %s 'a@uni.lk,b@uni.lk' | vercel env add ADMIN_EMAILS production --force`, then
   redeploy. A listed email becomes admin when it registers, or at its next login if the account
   already exists. Matching is case-insensitive. Removing an email from the list does **not**
   demote an existing admin.
2. **CLI**, from a trusted machine with the direct database URL:

   ```bash
   cd services/selar-api
   DATABASE_URL='postgres://…?sslmode=require' go run ./cmd/admin promote someone@example.com
   DATABASE_URL='…' go run ./cmd/admin demote someone@example.com
   DATABASE_URL='…' go run ./cmd/admin list
   ```

   The account must already exist, so register it in the console first.

### Group label

Admins can set a free-form **group label** on each user (Admin → Users → user → Group). It is
labelled neutrally and is never returned to the user, so a participant cannot see their own
group. The label exists so admins can filter and export by study condition. Changing it has no
effect on the app's behaviour. It is separate from the legacy `users.cohort` column, which is
still hard-coded at registration (see #14). **Protocol decision needed:** how participants are
allocated to groups, and who assigns the labels, belongs in the study protocol. This feature
only records the label.
