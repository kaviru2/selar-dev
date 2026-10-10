"use client";

import { useState } from "react";
import Link from "next/link";
import { clientFetch, type ChatMessage } from "@/lib/api";

/**
 * One feedback model per chat answer:
 *
 *  - "Was this helpful? 👍 / 👎" is the only rating. 👎 is recorded at once
 *    (it withdraws the answer's adaptive evidence) and then offers an
 *    optional short reason. A 👍 can still be changed to 👎; a 👎 is final,
 *    matching the server's sticky-negative rule.
 *  - "Report a wrong claim" is the one explicit correction: it needs a
 *    description, supersedes the answer and withdraws its evidence.
 *  - A rating is never evidence that two concepts are related. The only
 *    graph effect of 👍 is that cited passages are associated with concepts
 *    they mention; the single effect line says exactly that.
 *
 * API mapping is unchanged: 👍 → `helpful`, 👎 (+ reason) → `unhelpful`
 * with `correction_text`, report → `correction`. Comments stored on a 👍 by
 * the earlier UI are still shown, read-only.
 */
export const FEEDBACK_REASON_MAX = 500;

type Action = "helpful" | "unhelpful" | "correction";

export function feedbackState(message: ChatMessage) {
  const items = message.feedback ?? [];
  const unhelpful = items.find((item) => item.action === "unhelpful");
  const helpful = items.find((item) => item.action === "helpful");
  const reported = items.some((item) => item.action === "correction");
  return {
    rating: unhelpful ? "unhelpful" as const : helpful ? "helpful" as const : undefined,
    reason: unhelpful?.correction_text?.trim() || "",
    legacyNote: !unhelpful ? helpful?.correction_text?.trim() || "" : "",
    reported: reported || message.status === "superseded",
  };
}

export function AnswerFeedback({ message, onChanged }: { message: ChatMessage; onChanged: () => Promise<void> | void }) {
  const state = feedbackState(message);
  const [busy, setBusy] = useState<Action | "">("");
  const [error, setError] = useState("");
  const [editingReason, setEditingReason] = useState(false);
  const [reason, setReason] = useState("");
  const [reporting, setReporting] = useState(false);
  const [report, setReport] = useState("");

  async function send(action: Action, text = "") {
    setBusy(action);
    setError("");
    try {
      await clientFetch(`/api/chat/messages/${message.id}/feedback`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, correction_text: text.trim() }),
      });
      await onChanged();
      return true;
    } catch (sendError) {
      setError(sendError instanceof Error ? sendError.message : "Unable to record feedback");
      return false;
    } finally {
      setBusy("");
    }
  }

  const effect = <FeedbackEffect message={message} rating={state.rating} reported={state.reported} />;

  if (state.reported) {
    return (
      <div className="answer-feedback" role="group" aria-label="Answer feedback">
        <div className="answer-feedback-status">Reported as wrong · kept here for your record</div>
        {effect}
      </div>
    );
  }

  const showReasonBox = state.rating === "unhelpful" && (!state.reason || editingReason);
  return (
    <div className="answer-feedback" role="group" aria-label="Answer feedback">
      <div className="answer-feedback-row">
        <span>Was this helpful?</span>
        <button
          type="button"
          className={state.rating === "helpful" ? "selected" : ""}
          aria-label="Helpful"
          aria-pressed={state.rating === "helpful"}
          title="Helpful"
          disabled={busy !== "" || state.rating !== undefined}
          onClick={() => send("helpful")}
        >👍</button>
        <button
          type="button"
          className={state.rating === "unhelpful" ? "selected negative" : ""}
          aria-label="Not helpful"
          aria-pressed={state.rating === "unhelpful"}
          title={state.rating === "helpful" ? "Change to not helpful" : "Not helpful"}
          disabled={busy !== "" || state.rating === "unhelpful"}
          onClick={async () => { if (await send("unhelpful")) { setReason(""); setEditingReason(false); } }}
        >👎</button>
        <span className="answer-feedback-sep" aria-hidden="true">·</span>
        <button
          type="button"
          className="answer-feedback-report"
          aria-expanded={reporting}
          disabled={busy !== ""}
          onClick={() => setReporting((open) => !open)}
        >Report a wrong claim</button>
      </div>

      {showReasonBox && (
        <form className="answer-feedback-reason" aria-label="Reason for not helpful" onSubmit={async (event) => {
          event.preventDefault();
          if (reason.trim() && await send("unhelpful", reason)) setEditingReason(false);
        }}>
          <input
            aria-label="What was missing or off? (optional)"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            maxLength={FEEDBACK_REASON_MAX}
            placeholder="Optional: what was missing or off?"
          />
          <button type="submit" disabled={!reason.trim() || busy !== ""}>Add reason</button>
        </form>
      )}
      {state.rating === "unhelpful" && state.reason && !editingReason && (
        <div className="answer-feedback-note">
          Reason: {state.reason}{" "}
          <button type="button" onClick={() => { setReason(state.reason); setEditingReason(true); }}>Edit</button>
        </div>
      )}
      {state.legacyNote && <div className="answer-feedback-note">Your earlier note: {state.legacyNote}</div>}

      {reporting && (
        <form className="answer-feedback-report-form" aria-label="Report a wrong claim" onSubmit={async (event) => {
          event.preventDefault();
          if (report.trim() && await send("correction", report)) { setReporting(false); setReport(""); }
        }}>
          <label htmlFor={`report-${message.id}`}>Which claim is wrong, and what do your sources say instead?</label>
          <textarea id={`report-${message.id}`} value={report} onChange={(event) => setReport(event.target.value)} maxLength={2000} rows={3} />
          <small>
            The answer stays visible, marked as wrong, and stops shaping your graph and retrieval.
            To flag a connection between concepts, use “This link is wrong” on that link.
          </small>
          <div className="answer-feedback-actions">
            <button type="button" onClick={() => { setReporting(false); setReport(""); }}>Cancel</button>
            <button type="submit" className="primary" disabled={!report.trim() || busy !== ""}>Report as wrong</button>
          </div>
        </form>
      )}

      {effect}
      {error && <div className="chat-error" role="alert">{error}</div>}
    </div>
  );
}

function FeedbackEffect({ message, rating, reported }: {
  message: ChatMessage; rating?: "helpful" | "unhelpful"; reported: boolean;
}) {
  const update = message.graph_update;
  const hasEvidence = Boolean(update && (update.concepts_created > 0 || update.concepts_reinforced > 0));
  if (update?.retracted || reported || rating === "unhelpful") {
    return (
      <div className="answer-feedback-effect withdrawn" role="status">
        {hasEvidence ? "Evidence withdrawn — this answer no longer shapes" : "Marked not helpful — this answer won’t shape"} your graph or retrieval.
      </div>
    );
  }
  if (rating === "helpful" && update && hasEvidence) {
    const concepts = update.concepts_created + update.concepts_reinforced;
    return (
      <Link className="answer-feedback-effect" href="/graph">
        Cited passages associated with {concepts} concept{concepts === 1 ? "" : "s"}
        {update.concepts_created > 0 ? ` (${update.concepts_created} new candidate${update.concepts_created === 1 ? "" : "s"})` : ""}.
        {" "}A rating never shows that concepts are related. View graph →
      </Link>
    );
  }
  if (rating === "helpful") {
    return <div className="answer-feedback-effect" role="status">Thanks — recorded.</div>;
  }
  return null;
}
