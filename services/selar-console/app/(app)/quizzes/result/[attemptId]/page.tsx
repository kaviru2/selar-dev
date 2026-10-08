// /quizzes/result/[attemptId] — what the learner may see after submitting.
// The API decides what is visible (feedback policy); this page renders it.

import Link from "next/link";
import { getAuthToken } from "@/lib/auth";
import { serverFetch } from "@/lib/api";
import { Badge, Card, PageHeader } from "@/components/ui/Card";
import type { AttemptResult, ResultQuestion } from "@/lib/quiz/quiz";

export const dynamic = "force-dynamic";

const REASON: Record<AttemptResult["submit_reason"], string> = {
  learner: "You submitted this attempt.",
  time_limit: "Time ran out, so your saved answers were submitted automatically.",
  closed: "The quiz closed, so your saved answers were submitted automatically.",
};

function QuestionResult({ q }: { q: ResultQuestion }) {
  const status = q.pending_review ? <Badge tone="amber">Awaiting review</Badge>
    : q.correct === true ? <Badge tone="green">Correct</Badge>
    : q.correct === false ? <Badge tone="warm">Not correct</Badge> : null;
  return (
    <Card as="li" className="quiz-result-q">
      <div className="quiz-result-q-head">
        <span className="quiz-result-num">{q.index + 1}</span>
        <h3>{q.prompt}</h3>
        {status}
        {q.score != null && <span className="quiz-result-pts">{q.score} / {q.points}</span>}
      </div>
      {q.cue && <p className="quiz-cue"><span>Cue:</span> {q.cue}</p>}
      {q.options && (
        <ul className="quiz-result-options">
          {q.options.map((o) => (
            <li key={o.id} className={`${o.correct ? "is-correct" : ""} ${o.selected ? "is-selected" : ""}`}>
              <span>{o.text}</span>
              {o.selected && <span className="quiz-tag">Your answer</span>}
              {o.correct && <span className="quiz-tag quiz-tag--ok">Correct answer</span>}
            </li>
          ))}
        </ul>
      )}
      {!q.options && (
        <div className="quiz-result-text">
          <p className="quiz-label">Your answer</p>
          <p>{q.text?.trim() ? q.text : <em>No answer</em>}</p>
          {q.accepted_answers && q.accepted_answers.length > 0 && <p className="quiz-accepted">Accepted: {q.accepted_answers.join(", ")}</p>}
        </div>
      )}
      {q.explanation && <p className="quiz-explanation">{q.explanation}</p>}
      {q.grader_note && <p className="quiz-explanation"><strong>Reviewer note:</strong> {q.grader_note}</p>}
      {(q.source_document || q.source_url) && (
        <p className="quiz-source">Source: {q.source_url ? <a href={q.source_url} target="_blank" rel="noreferrer">{q.source_document || q.source_url}</a> : q.source_document}</p>
      )}
    </Card>
  );
}

export default async function ResultPage({ params }: { params: Promise<{ attemptId: string }> }) {
  const { attemptId } = await params;
  const token = await getAuthToken();
  let res: AttemptResult | null = null;
  let error: string | null = null;
  try {
    res = token ? await serverFetch<AttemptResult>(`/api/quiz-attempts/${encodeURIComponent(attemptId)}/result`, token) : null;
  } catch (e) {
    error = e instanceof Error ? e.message : "Result unavailable";
  }
  if (!res) {
    return (
      <main className="quiz-page quiz-page--narrow">
        <PageHeader eyebrow="Quizzes" title="Result unavailable" description={error ?? undefined} />
        <Link href="/quizzes">Back to quizzes</Link>
      </main>
    );
  }
  const s = res.summary;
  return (
    <main className="quiz-page quiz-page--narrow">
      <PageHeader eyebrow="Quiz submitted" title={res.title} description={REASON[res.submit_reason]} />
      {!res.feedback_available ? (
        <Card className="quiz-result-hidden">
          <h2>Thank you, your answers are saved.</h2>
          <p>
            {res.feedback_policy === "after_close"
              ? "Results will be shown here after the quiz closes."
              : "Results are not shown for this quiz."}
          </p>
        </Card>
      ) : (
        <>
          {s && (
            <Card tint={s.pending_review > 0 ? "none" : "green"} className="quiz-score">
              {s.score != null ? (
                <p className="quiz-score-big">{s.score} <span>/ {s.max_points}</span>{s.percent != null && <small>{Math.round(s.percent)}%</small>}</p>
              ) : (
                <p className="quiz-score-big">{s.auto_points} <span>/ {s.max_points} so far</span></p>
              )}
              {s.pending_review > 0 && <p>{s.pending_review} answer{s.pending_review === 1 ? " is" : "s are"} waiting for a reviewer. Your final score will appear here.</p>}
            </Card>
          )}
          {res.questions && <ol className="quiz-result-list">{res.questions.map((q) => <QuestionResult key={q.id} q={q} />)}</ol>}
        </>
      )}
      <p><Link href="/quizzes">Back to quizzes</Link></p>
    </main>
  );
}
