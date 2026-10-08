"use client";

// GradeAttempt — score free-text answers against the rubric. Each save goes
// straight to the API, which recomputes the attempt score.

import { useState } from "react";
import { Badge, Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { quizApi } from "@/lib/quiz/client";
import { formatDuration, QUESTION_TYPES, type Summary } from "@/lib/quiz/quiz";

export interface AdminAnswer {
  id: string; question_id: string; index: number; type: string; prompt: string; cue?: string; rubric?: string;
  accepted_answers?: string[]; options?: { id: string; text: string; correct?: boolean }[]; points: number;
  selected: string[]; text: string; auto_score?: number; manual_score?: number; is_correct?: boolean;
  needs_review: boolean; grader_note: string; time_on_question_ms: number; revisions: number;
}
export interface AdminAttemptDetail { id: string; quiz_id: string; quiz_title: string; submitted_at?: string; summary: Summary; answers: AdminAnswer[] }

const typeLabel = (t: string) => QUESTION_TYPES.find((x) => x.value === t)?.label ?? t;

function AnswerCard({ a, attemptId, onSaved }: { a: AdminAnswer; attemptId: string; onSaved: (a: AdminAnswer, s: Summary) => void }) {
  const [score, setScore] = useState<string>(a.manual_score != null ? String(a.manual_score) : "");
  const [note, setNote] = useState(a.grader_note ?? "");
  const [state, setState] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [error, setError] = useState<string | null>(null);
  const objective = !!a.options;
  const save = async () => {
    const n = Number(score);
    if (score === "" || Number.isNaN(n) || n < 0 || n > a.points) { setError(`Enter a score from 0 to ${a.points}`); return; }
    setError(null); setState("saving");
    try {
      await quizApi(`/api/admin/quiz-answers/${a.id}/grade`, { method: "PUT", body: JSON.stringify({ score: n, note }) });
      const fresh = await quizApi<AdminAttemptDetail>(`/api/admin/quiz-attempts/${attemptId}`);
      onSaved(fresh.answers.find((x) => x.id === a.id) ?? { ...a, manual_score: n, grader_note: note, needs_review: false }, fresh.summary);
      setState("saved");
    } catch (e) { setError(e instanceof Error ? e.message : "Save failed"); setState("error"); }
  };
  const effective = a.manual_score ?? a.auto_score;
  return (
    <Card as="li" className="aq-grade">
      <div className="quiz-result-q-head">
        <span className="quiz-result-num">{a.index + 1}</span>
        <h3>{a.prompt}</h3>
        <Badge tone={a.needs_review ? "amber" : "green"}>{a.needs_review ? "To grade" : `${effective ?? 0} / ${a.points}`}</Badge>
      </div>
      <p className="aq-muted">{typeLabel(a.type)} · {formatDuration(a.time_on_question_ms)} on question · {a.revisions} edit{a.revisions === 1 ? "" : "s"}</p>
      {a.cue && <p className="quiz-cue"><span>Cue:</span> {a.cue}</p>}
      {objective ? (
        <ul className="quiz-result-options">
          {a.options!.map((o) => (
            <li key={o.id} className={`${o.correct ? "is-correct" : ""} ${a.selected.includes(o.id) ? "is-selected" : ""}`}>
              <span>{o.text}</span>{a.selected.includes(o.id) && <span className="quiz-tag">Chosen</span>}{o.correct && <span className="quiz-tag quiz-tag--ok">Key</span>}
            </li>
          ))}
        </ul>
      ) : (
        <blockquote className="aq-answer">{a.text?.trim() ? a.text : <em>No answer</em>}</blockquote>
      )}
      {a.accepted_answers && a.accepted_answers.length > 0 && <p className="aq-muted">Accepted: {a.accepted_answers.join(" · ")}</p>}
      {a.rubric && <p className="aq-rubric"><strong>Rubric:</strong> {a.rubric}</p>}
      <div className="aq-grade-form">
        <label>Score <input type="number" min={0} max={a.points} step={0.5} value={score} onChange={(e) => setScore(e.target.value)} aria-label={`Score for question ${a.index + 1}`} /> / {a.points}</label>
        <label className="aq-grade-note">Note (visible to the learner if results are shown)
          <input value={note} onChange={(e) => setNote(e.target.value)} aria-label={`Note for question ${a.index + 1}`} />
        </label>
        <Button size="sm" variant={a.needs_review ? "primary" : "secondary"} onClick={save} disabled={state === "saving"}>
          {objective ? "Override" : "Save score"}
        </Button>
        <span aria-live="polite" className="aq-muted">{state === "saved" ? "Saved" : ""}</span>
      </div>
      {error && <p role="alert" className="quiz-error">{error}</p>}
    </Card>
  );
}

export function GradeAttempt({ detail }: { detail: AdminAttemptDetail }) {
  const [answers, setAnswers] = useState(() => [...detail.answers].sort((x, y) => x.index - y.index));
  const [summary, setSummary] = useState(detail.summary);
  const [onlyPending, setOnlyPending] = useState(detail.summary.pending_review > 0);
  const shown = onlyPending ? answers.filter((a) => a.needs_review) : answers;
  return (
    <>
      <Card tint="green" className="quiz-score">
        <p className="quiz-score-big">
          {summary.score != null ? <>{summary.score} <span>/ {summary.max_points}</span></> : <>{summary.auto_points} <span>/ {summary.max_points} so far</span></>}
        </p>
        <p>{summary.pending_review ? `${summary.pending_review} answer${summary.pending_review === 1 ? "" : "s"} still to grade.` : "Fully graded."}</p>
        <label className="aq-inline-check"><input type="checkbox" checked={onlyPending} onChange={(e) => setOnlyPending(e.target.checked)} /> Show only answers to grade</label>
      </Card>
      <ol className="quiz-result-list">
        {shown.map((a) => (
          <AnswerCard key={a.id} a={a} attemptId={detail.id} onSaved={(na, s) => { setAnswers((xs) => xs.map((x) => (x.id === na.id ? na : x))); setSummary(s); }} />
        ))}
      </ol>
      {shown.length === 0 && <p className="aq-muted">Nothing left to grade in this attempt.</p>}
    </>
  );
}
