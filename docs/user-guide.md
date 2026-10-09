# Using SELAR

[README](../README.md) · [Run locally](local-quickstart.md)

SELAR is a research prototype for reading across sources and checking suggested connections. It has not established that using it improves memory or retention. Treat AI answers and links as suggestions to verify.

## Open your library

1. Open the hosted address supplied by your operator, or the console address from the local quickstart.
2. Register or sign in. Google sign-in appears only on instances configured for it; a Google Drive connection is a separate optional capability.
3. Read the processing and analytics disclosures. Use non-sensitive test readings, not confidential, personal or participant data. Local hosting alone does not keep model processing on your computer.
4. Go to **Library → Add content**.

## Supported inputs

This table describes the baseline reviewed at commit `f80d5233fc9ca5e6737bc1b285ec95a68a518e8a`. New format and onboarding work must be merged and verified before it is treated as available.

| Input | How to add it | Boundary |
|---|---|---|
| PDF on your computer | **Upload PDF** | Up to 10 PDFs per batch, 50 MB per file. Start with one small, text-based PDF. Extraction quality varies. |
| Public article or blog | **From the web** | Publicly fetchable pages only; this does not bypass logins, paywalls or site access controls. |
| Notes or Markdown text | **Paste text** | Paste the content into the editor. Markdown support is partial; this is not a `.md`/`.txt` file-upload feature. |
| PDF in Google Drive | Optional **Import from Google Drive** control | Requires operator configuration and your authorization. PDF only in this baseline; native Google Docs are not exported. |
| DOCX, native Google Docs, `.md`/`.txt` files | Not supported as direct imports in this baseline | Export to PDF yourself, or paste permitted text. Do not just rename a file's extension. |

An upload being accepted does not mean processing has finished. Wait for the reading to become ready before expecting citations or connections. Processing uses the instance's worker and model quota. Two related readings are a useful starting point for cross-document links, but no link is guaranteed.

## A first reading session

1. Open a ready reading in **Reader**.
2. Read a passage before looking at suggested connections. Suggestions may be hidden or locked by the instance's study settings.
3. When a connection is available, choose **Think about this link**. You can explain it in your own words or skip for now.
4. Choose **Show me the passages** and compare both source excerpts and their locations. If either excerpt is unavailable, do not infer that the connection is valid.
5. Choose **Yes, keep this link**, **Different relationship**, or **Not a real link**. Relabeling and rejecting ask for a note or reason. **Skip for now** is also valid.
6. Revisit **Kept links** or **Graph** to inspect what you retained. A graph relationship is not proof of understanding or a measured retention score.

The optional explanation/recall text in this comparison flow is described by the current UI as unsaved. That does **not** mean all activity is ephemeral: kept-link decisions, relationship notes, chat and other account records are separate stored data.

## Ask a question and check the answer

Open **Chat**, ask about material already processed into your library, then open the cited passages. Check that the source really supports the answer, and use feedback controls when it does not. Missing evidence, incomplete extraction and model errors are all possible. Chat is not a substitute for reading the source.

**Quizzes** contains assigned quizzes when an operator has made them available. It is not the pending self-directed practice feature. Account settings, data controls and optional integrations are under **Settings**; see the operator documentation for [quizzes](QUIZZES.md), [analytics](ANALYTICS.md) and [reminders](REMINDERS.md).

## If something looks wrong

| Symptom | What to do |
|---|---|
| A reading stays queued or processing | Wait briefly, then ask the operator to check the worker and model quota. Self-hosters: check the [queue and storage configuration](local-quickstart.md#troubleshooting). Avoid repeatedly uploading the same document. |
| Import fails | Check the supported-input table, file limit and page accessibility. Read the displayed error; use retry when offered after the cause is fixed. |
| No connections | Check that readings are ready and related. A blank result can be legitimate; suggestion visibility may also be restricted by the instance. |
| Google or Drive controls are missing | They are optional and not enabled by the local smoke setup. Ask the instance operator rather than sharing credentials. |
| An answer or connection is wrong | Inspect the cited passages and reject or report it. Do not treat generated wording or confidence as source evidence. |
| Your screen differs from this guide | Ask which revision is deployed. Pending work is not necessarily in the hosted instance. |

When reporting a bug, include the action, error text and deployed revision if known. Do not attach secrets, sensitive readings or other users' information.

## Pending, not promised

At this guide's baseline, PRs #141–#145 propose private document practice, reading checks, spaced review/daily sessions, reflection links and progress reporting. Format and onboarding extensions are also being developed separately. These are not instructions for features already shipped here; the guide should be updated after integration and runtime verification.
