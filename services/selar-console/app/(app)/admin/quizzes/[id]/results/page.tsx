// /admin/quizzes/[id]/results — score summaries, per-question stats, CSV export,
// and links into blind manual grading.

import Link from "next/link";
import { notFound } from "next/navigation";
import { serverFetch } from "@/lib/api";
import { requireAdminPage } from "@/lib/quiz/admin-gate";
import { Badge, Card, PageHeader } from "@/components/ui/Card";
import { formatDuration, QUESTION_TYPES, type QuizRecord } from "@/lib/quiz/quiz";

export const dynamic = "force-dynamic";

interface AttemptRow {
  id: string; user_id: string; email: string; cohort: string; attempt_number: number; quiz_version: number;
  started_at: string; submitted_at?: string; submit_reason?: string; duration_ms?: number;
  max_points: number; auto_points: number; pending_review: number; score?: number;
}
interface Stat {
  question_id: string; position: number; type: string; prompt: string; points: number; answered: number; blank: number;
  pending_review: number; mean_score?: number; percent_correct?: number; median_time_ms?: number;
}

const typeLabel = (t: string) => QUESTION_TYPES.find((x) => x.value === t)?.label ?? t;
const fmt = (n?: number | null, d = 1) => (n == null ? "–" : Number(n.toFixed(d)).toString());

export default async function ResultsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const token = await requireAdminPage();
  const enc = encodeURIComponent(id);
  let record: QuizRecord, attempts: AttemptRow[], stats: Stat[];
  try {
    [record, attempts, stats] = await Promise.all([
      serverFetch<QuizRecord>(`/api/admin/quizzes/${enc}`, token),
      serverFetch<AttemptRow[]>(`/api/admin/quizzes/${enc}/attempts`, token),
      serverFetch<Stat[]>(`/api/admin/quizzes/${enc}/stats`, token),
    ]);
  } catch {
    notFound();
  }
  const submitted = attempts.filter((a) => a.submitted_at);
  const scored = submitted.filter((a) => a.score != null);
  const pending = submitted.reduce((n, a) => n + a.pending_review, 0);
  const mean = scored.length ? scored.reduce((s, a) => s + (a.score! / (a.max_points || 1)) * 100, 0) / scored.length : null;
  const firstPending = submitted.find((a) => a.pending_review > 0);

  return (
    <main className="quiz-page aq-page">
      <p className="aq-crumb"><Link href={`/admin/quizzes/${id}`}>← {record.title}</Link></p>
      <PageHeader
        eyebrow="Results"
        title={record.title}
        actions={
          <span className="aq-row">
            <a className="ui-btn" href={`/api/admin/quizzes/${enc}/results.csv?level=attempt`} download>CSV: per attempt</a>
            <a className="ui-btn" href={`/api/admin/quizzes/${enc}/results.csv?level=answer`} download>CSV: per question</a>
          </span>
        }
      />
      <div className="aq-kpis">
        <Card><p className="aq-kpi">{submitted.length}</p><p className="aq-muted">submitted</p></Card>
        <Card><p className="aq-kpi">{attempts.length - submitted.length}</p><p className="aq-muted">in progress</p></Card>
        <Card><p className="aq-kpi">{mean == null ? "–" : `${Math.round(mean)}%`}</p><p className="aq-muted">mean score (fully graded)</p></Card>
        <Card tint={pending ? "warm" : "none"}>
          <p className="aq-kpi">{pending}</p>
          <p className="aq-muted">answers to grade{firstPending && <> · <Link href={`/admin/quiz-attempts/${firstPending.id}`}>start grading</Link></>}</p>
        </Card>
      </div>

      <section aria-labelledby="aq-stats-h">
        <h2 id="aq-stats-h">By question</h2>
        <div className="aq-table-wrap">
          <table className="aq-table">
            <thead><tr><th scope="col">#</th><th scope="col">Question</th><th scope="col">Type</th><th scope="col">Answered</th><th scope="col">Blank</th><th scope="col">% correct</th><th scope="col">Mean score</th><th scope="col">Median time</th><th scope="col">To grade</th></tr></thead>
            <tbody>
              {stats.map((s) => (
                <tr key={s.question_id}>
                  <td>{s.position + 1}</td>
                  <td className="aq-cell-prompt">{s.prompt}</td>
                  <td>{typeLabel(s.type)}</td>
                  <td>{s.answered}</td>
                  <td>{s.blank}</td>
                  <td>{s.percent_correct == null ? "–" : `${Math.round(s.percent_correct)}%`}</td>
                  <td>{fmt(s.mean_score)} / {s.points}</td>
                  <td>{s.median_time_ms == null ? "–" : formatDuration(s.median_time_ms)}</td>
                  <td>{s.pending_review || ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section aria-labelledby="aq-att-h">
        <h2 id="aq-att-h">Attempts</h2>
        {attempts.length === 0 ? <p className="aq-muted">No attempts yet.</p> : (
          <div className="aq-table-wrap">
            <table className="aq-table">
              <thead><tr><th scope="col">Learner</th><th scope="col">Cohort</th><th scope="col">#</th><th scope="col">Submitted</th><th scope="col">Time</th><th scope="col">Score</th><th scope="col"><span className="sr-only">Grade</span></th></tr></thead>
              <tbody>
                {attempts.map((a) => (
                  <tr key={a.id}>
                    <td>{a.email}</td>
                    <td>{a.cohort}</td>
                    <td>{a.attempt_number}{a.quiz_version !== record.version ? ` (v${a.quiz_version})` : ""}</td>
                    <td>
                      {a.submitted_at ? new Date(a.submitted_at).toLocaleString() : <Badge tone="sky">in progress</Badge>}
                      {a.submit_reason && a.submit_reason !== "learner" && <> <Badge tone="amber">{a.submit_reason.replace("_", " ")}</Badge></>}
                    </td>
                    <td>{a.duration_ms != null ? formatDuration(a.duration_ms) : "–"}</td>
                    <td>{a.score != null ? `${fmt(a.score)} / ${a.max_points}` : a.submitted_at ? `${fmt(a.auto_points)} + ${a.pending_review} to grade` : "–"}</td>
                    <td>{a.submitted_at && <Link href={`/admin/quiz-attempts/${a.id}`}>{a.pending_review ? "Grade" : "Review"}</Link>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </main>
  );
}
