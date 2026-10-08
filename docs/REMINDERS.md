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
