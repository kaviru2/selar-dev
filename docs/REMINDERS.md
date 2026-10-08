# Reminders: quiz-window calendar and email notices

SELAR can remind participants about quiz windows in two optional ways. **Both
are off by default and send nothing until the research team switches them on.**
Reminders can change how participants behave in a study, so switching them on
is a study-protocol decision, not a deployment detail. When they are on, they
are identical for every study arm.

## Quiz-window calendar (issue #113)

A participant who opts in under **Settings › Reminders** gets a private
calendar link (`https://<api>/calendar/<token>.ics`). Google Calendar subscribes
to it with *Other calendars › From URL*. The Settings page also has an "Add to
Google Calendar" button and a one-off "Add this one" link for each window.

How it works:

- **No Google API and no OAuth.** The API serves a standard iCalendar feed.
  Google fetches it like any URL. Nothing is written to anyone's Google
  account, so no consent screen or scope is involved, and there is no cost.
- **Windows match the server.** Each fetch evaluates every published quiz
  with `quiz.Evaluate`, the same function that enforces availability. That
  covers absolute windows and relative ones ("7 days after first reading").
  Only upcoming, open and in-progress windows appear. A submitted or missed
  quiz drops out. A relative window appears only after its anchor event.
- **Time zones.** Every time is written in UTC (`…Z`). Calendar apps show it in
  the viewer's zone, so a 09:00–17:30 Asia/Colombo window appears as exactly
  09:00–17:30 in Colombo. Tests cover this case.
- **Updates.** Each event's UID is stable per participant and quiz, so a
  changed window replaces the old event. Google refreshes subscribed calendars
  on its own schedule, usually every few hours, and this cannot be forced. The
  feed asks for hourly refresh.
- **No study-arm information.** Events show only the quiz kind ("Initial
  test", "Follow-up test", "Practice quiz"), the times and a link to
  `/quizzes`. Quiz titles, descriptions, cohort and group labels are never
  included. UIDs hash the user id.
- **Revocable links.** The token is an HMAC of (user id, feed version). The key
  comes from `LINK_TOKEN_SECRET`, or from `JWT_SECRET` if that is not set.
  Nothing is stored. *Reset link* and turning the calendar off increment the
  version, so earlier links stop working. A revoked link still returns a valid
  but empty calendar, which makes subscribed apps remove the old events. A
  forged token gets 404.

### Switches (API environment)

| Variable | Default | Meaning |
|---|---|---|
| `CALENDAR_FEED_ENABLED` | unset (off) | Master switch. Only `true`/`1`/`yes`/`on` turns it on. |
| `CALENDAR_FEED_ALLOWLIST` | unset (nobody) | Accounts that may use it: addresses, `@domain`, or `*` for everyone. |
| `PUBLIC_API_URL` | request host | External API origin used in feed links, e.g. `https://selar-api.vercel.app`. |
| `PUBLIC_CONSOLE_URL` | first `CORS_ORIGIN` | Console origin used in event links. |
| `LINK_TOKEN_SECRET` | `JWT_SECRET` | Optional separate key for signed links. Changing it revokes every calendar link. |

A participant sees the toggle only when the master switch is on **and** their
address matches the allowlist. Otherwise Settings says the feature is not
switched on for this study, and every feed URL returns an empty calendar.
Turning the master switch off later empties every feed on the next refresh.

## Email study notices (issue #114)

The API can email three kinds of notice:

- **A quiz is open.** Sent when a window opens, but only within 6 hours of the
  opening time. A late-joining learner is never told about a window that
  opened long ago.
- **A quiz closes soon.** Sent within the last 24 hours of a window. Windows
  shorter than 24 hours get only the opening notice.
- **Account security.** Sent when the password or sign-in email changes. An
  email-change notice goes to the *previous* address.

Templates live in `services/selar-api/internal/notify/templates.go`. They are
plain text and say SELAR is a research prototype. They make no claims about
memory, learning or grades, include no quiz title, cohort or group, and are
word-for-word the same for every study arm. Tests check all of these.

### What stops a real email from going out

The default configuration sends nothing. Every message goes through a
`Transport`:

| Condition | Result |
|---|---|
| Account not in `NOTIFY_EMAIL_ALLOWLIST` (default: empty) | No notice at all, and Settings shows "not switched on for this study". |
| Allowlisted, learner opted in, `NOTIFY_EMAIL_ENABLED` unset or false | **Dry-run.** An audit row is written with `transport='dry-run'` and the API log gets a redacted line. Nothing is sent. |
| Above, plus `NOTIFY_EMAIL_ENABLED=true` but SMTP incomplete | Dry-run. The startup log says SMTP is incomplete. |
| Above, plus complete `NOTIFY_SMTP_*` | Sent over SMTP with STARTTLS or implicit TLS. Without TLS the transport refuses to send. |

Other protections:

- **Opt-in per learner.** Quiz emails and security emails are separate
  switches under **Settings › Reminders**, both off by default. Opting out
  always works.
- **Unsubscribe.** Every email has a signed link, plus `List-Unsubscribe` and
  one-click (RFC 8058) headers. A GET only shows a confirmation button, since
  mail scanners open links. The POST turns every notice off and invalidates
  the link.
- **Idempotency.** Each notice claims a unique `dedupe_key` in
  `notification_log` (kind, learner, quiz, window start and end) *before*
  sending. Concurrent or repeated runs cannot double-send. A failed send is
  recorded and not retried for that window. If an admin moves a window, it
  counts as a new window and can be announced once more.
- **Rate limits.** At most 3 notices per learner per 24 hours, 200 in total
  per 24 hours, and 50 per run.
- **Audit.** `notification_log` (migration 019, additive) stores the kind,
  quiz, window, transport, subject, status and a salted hash of the recipient,
  never the address. Rows are deleted with the account and included in
  *Export my data*. Admins can read `GET /api/admin/notifications/log`.

**Why not the Gmail API.** Sending from a mailbox through the Gmail API needs an
OAuth refresh token for that mailbox (`gmail.send` scope). By decision this is
not set up for Kaviru's personal mailbox. The SMTP transport works with a
dedicated project mailbox (Gmail or Workspace app password) or any provider.
A Gmail-API transport can be added behind the same `Transport` interface if the
team later creates a dedicated sender account.

**Scheduling.** Modal's `notifications` function (every 15 minutes, see
`services/selar-worker/modal_app.py`) POSTs to `/internal/notifications/run`
with `X-Selar-Worker-Secret`. It does nothing unless `SELAR_API_URL` is set in
the Modal secret.

### Switches (API environment unless noted)

| Variable | Default | Meaning |
|---|---|---|
| `NOTIFY_EMAIL_ALLOWLIST` | empty | Who may opt in and receive notices: addresses, `@domain`, or `*`. |
| `NOTIFY_EMAIL_ENABLED` | unset | Only `true` permits real sending. Without it everything is dry-run. |
| `NOTIFY_SMTP_HOST`, `NOTIFY_SMTP_PORT` (587 or 465), `NOTIFY_SMTP_USERNAME`, `NOTIFY_SMTP_PASSWORD`, `NOTIFY_EMAIL_FROM`, `NOTIFY_EMAIL_REPLY_TO` | unset | SMTP sender. All except reply-to are required. |
| `NOTIFY_TIMEZONE` | `Asia/Colombo` | Time zone used for times in the email text. |
| `PUBLIC_API_URL` | none | Needed for unsubscribe links. Without it, emails have no link (so do not enable live sending without it). |
| `WORKER_TRIGGER_SECRET` | existing | Authenticates the scheduler call. |
| `SELAR_API_URL` (Modal secret) | unset | API origin the schedule calls. Unset means the schedule is idle. |

### Switching on, step by step (for the research team)

1. Agree the protocol: which notices, for which phase, identical for all arms,
   and whether the ethics approval covers contacting participants by email.
2. Set `PUBLIC_API_URL` and `NOTIFY_EMAIL_ALLOWLIST=<one team test address>`,
   and leave `NOTIFY_EMAIL_ENABLED` unset. Opt that account in, run the
   schedule, and read `GET /api/admin/notifications/log`. Every row should say
   `dry-run`.
3. To send for real, set the `NOTIFY_SMTP_*` variables and
   `NOTIFY_EMAIL_ENABLED=true`, still with the one-address allowlist, and
   check a real message arrives.
4. Widen `NOTIFY_EMAIL_ALLOWLIST` to the participant list or `*`.
   Participants still have to opt in themselves.
