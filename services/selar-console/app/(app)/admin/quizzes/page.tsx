// /admin/quizzes — list quizzes, import a file, open the editor.

import Link from "next/link";
import { serverFetch } from "@/lib/api";
import { requireAdminPage } from "@/lib/quiz/admin-gate";
import { Badge, PageHeader } from "@/components/ui/Card";
import { ButtonLink } from "@/components/ui/Button";
import { AdminQuizImport } from "./AdminQuizImport";
import { KIND_LABEL, type AdminQuizSummary } from "@/lib/quiz/quiz";

export const dynamic = "force-dynamic";

const STATUS_TONE = { draft: "neutral", published: "green", closed: "warm" } as const;

export default async function AdminQuizzesPage() {
  const token = await requireAdminPage();
  const quizzes = await serverFetch<AdminQuizSummary[]>("/api/admin/quizzes", token).catch(() => [] as AdminQuizSummary[]);
  return (
    <main className="quiz-page aq-page">
      <PageHeader
        eyebrow="Admin"
        title="Quizzes"
        description="Import a quiz file to deploy in one step, or build one in the editor."
        actions={<ButtonLink href="/admin/quizzes/new" variant="primary">New quiz</ButtonLink>}
      />
      <p className="aq-banner aq-banner--info">
        Use only invented or approved content. The study&apos;s initial and day-7 items need supervisor and ethics sign-off before they are entered.
      </p>
      <div className="aq-layout">
        <section aria-labelledby="aq-list-h" className="aq-list">
          <h2 id="aq-list-h">All quizzes</h2>
          {quizzes.length === 0 ? <p className="aq-muted">No quizzes yet.</p> : (
            <div className="aq-table-wrap">
              <table className="aq-table">
                <thead>
                  <tr><th scope="col">Title</th><th scope="col">Kind</th><th scope="col">Status</th><th scope="col">Questions</th><th scope="col">Submitted</th><th scope="col">To grade</th><th scope="col"><span className="sr-only">Actions</span></th></tr>
                </thead>
                <tbody>
                  {quizzes.map((q) => (
                    <tr key={q.id}>
                      <td><Link href={`/admin/quizzes/${q.id}`}>{q.title}</Link></td>
                      <td>{KIND_LABEL[q.kind]}</td>
                      <td><Badge tone={STATUS_TONE[q.status]}>{q.status}</Badge></td>
                      <td>{q.question_count}</td>
                      <td>{q.submitted}{q.attempts > q.submitted ? ` (+${q.attempts - q.submitted} open)` : ""}</td>
                      <td>{q.pending_review > 0 ? <Badge tone="amber">{q.pending_review}</Badge> : "0"}</td>
                      <td><Link href={`/admin/quizzes/${q.id}/results`}>Results</Link></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
        <AdminQuizImport />
      </div>
    </main>
  );
}
