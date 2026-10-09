# Using SELAR

[README](../README.md) · [Run locally](local-quickstart.md)

SELAR is a research prototype for reading across sources and checking suggested connections. It has not established that using it improves memory or retention. Treat AI answers and links as suggestions to verify.

## Open your library

1. Open the hosted address supplied by your operator, or the console address from the local quickstart.
2. Register or sign in. Google sign-in appears only on instances configured for it; a Google Drive connection is a separate optional capability.
3. Read the processing and analytics disclosures. Use non-sensitive test readings, not confidential, personal or participant data. Local hosting alone does not keep model processing on your computer.
4. Go to **Library → Add content**.

## Supported inputs

This guide describes merged application baseline `ce0ba3c542e3fa0ab1d804033407220d8cf57144`, not a deployed release. Expanded format and lightweight onboarding work remain separate; ask the operator which revision is running.

| Input | How to add it | Boundary |
|---|---|---|
| PDF on your computer | **Upload PDF** | Up to 10 PDFs per batch, 50 MB per file. Start with one small, text-based PDF. Extraction quality varies. |
| Public article or blog | **From the web** | Publicly fetchable pages only; this does not bypass logins, paywalls or site access controls. |
| Notes or Markdown text | **Paste text** | Paste the content into the editor. Markdown support is partial; this is not a `.md`/`.txt` file-upload feature. |
| PDF in Google Drive | Optional **Import from Google Drive** control | Requires operator configuration and your authorization. PDF only in this baseline; native Google Docs are not exported. |
| DOCX, native Google Docs, `.md`/`.txt` files | Not supported as direct imports in this baseline | Export to PDF yourself, or paste permitted text. Do not just rename a file's extension. |

An upload being accepted does not mean processing has finished. Wait for the reading to become ready before expecting citations or connections. Processing uses the instance's worker and model quota. Two related readings are a useful starting point for cross-document links, but no link is guaranteed.

## A first reading session

1. Open a ready reading in **Reader**. The **Before reading** screen requests generated practice automatically; this can use cloud quota even if you skip answering.
2. Optionally type what you remember and select **Check my recall**. Mark **I used the source or a hint** whenever applicable. Select **Continue reading** to skip or move on; unavailable practice does not block reading.
3. In **Learning connections**, reflect on similarities or differences, then choose **Compare passages**. Use **Open passage** to inspect each location. Prompts require two current, exact located passages; source matching does not prove a relationship.
4. Use **This link is wrong** to hide or retract an unsuitable prompt. There is no required keep/relabel/reject decision. Opening or flagging prompts does not count as recall success or create a graph assertion; historical graph records remain available.
5. Select **End-reading check**, answer from recall, and inspect the AI feedback, reference answer, quote and locator. These immediate checks are recorded as source-exposed, not delayed retention. **Continue reading** returns to the source.

The optional connection reflection is not graded, saved or sent anywhere. Practice answers, feedback and account activity are separate stored records. Generated practice is scoped to your account and checked in a separate model call for source support before use; answer grading is another AI step. These calls use the configured provider, not an independent human verifier. Feedback can be provisional/unscored and does not establish correctness or mastery.

## Review and progress

Open **Review** (`/review`) or **Daily review** after completing an end-reading check. Checks schedule practice for later; an empty queue can simply mean nothing is due. Recall before opening the source, mark hint/source use honestly, select **Check my recall**, then **Next question**. Visiting or skipping a question is not a successful recall attempt.

The schedule is a conservative doubling-interval heuristic, not FSRS or a calibrated memory model. Exposed, unscored or lower-scoring attempts return to a one-day interval; qualifying review scores can extend intervals up to 60 days. Review streaks use **UTC calendar days**, which may differ from your local day.

Open **Progress** (`/progress`) for warm-up, immediate-check and review activity, source-exposed and unscored counts, due items and review streaks. Delayed unassisted attempts are distinguished from immediate/exposed activity. These are observed practice records, not estimated memory strength, proven retention gains or a mastery score.

## Ask a question and check the answer

Open **Chat**, ask about material already processed into your library, then open the cited passages. Check that the source really supports the answer, and use feedback controls when it does not. Missing evidence, incomplete extraction and model errors are all possible. Chat is not a substitute for reading the source.

**Quizzes** contains assigned quizzes when an operator has made them available. It remains separate from self-directed generated practice. Account settings, data controls and optional integrations are under **Settings**; see the operator documentation for [quizzes](QUIZZES.md), [analytics](ANALYTICS.md) and [reminders](REMINDERS.md).

## If something looks wrong

| Symptom | What to do |
|---|---|
| A reading stays queued or processing | Wait briefly, then ask the operator to check the worker and model quota. Self-hosters: check the [queue and storage configuration](local-quickstart.md#troubleshooting). Avoid repeatedly uploading the same document. |
| Import fails | Check the supported-input table, file limit and page accessibility. Read the displayed error; use retry when offered after the cause is fixed. |
| No connections | Check that readings are ready and related. A blank result can be legitimate; suggestion visibility may also be restricted by the instance. |
| Google or Drive controls are missing | They are optional and not enabled by the local smoke setup. Ask the instance operator rather than sharing credentials. |
| Practice unavailable or provisional | Continue reading, or use **Retry practice** after ingestion/provider availability is fixed. Provisional/unscored feedback is not a failed study test. |
| An answer or connection is wrong | Inspect the cited passages; use **This link is wrong** for a connection or chat feedback for an answer. Do not treat generated wording or confidence as source evidence. |
| Your screen differs from this guide | Ask which revision is deployed. Pending work is not necessarily in the hosted instance. |

When reporting a bug, include the action, error text and deployed revision if known. Do not attach secrets, sensitive readings or other users' information.

## Pending, not promised

Expanded file imports and lightweight onboarding are being developed separately and are not part of this guide's merged baseline. Use the supported-input table above rather than assuming those branches are deployed.
