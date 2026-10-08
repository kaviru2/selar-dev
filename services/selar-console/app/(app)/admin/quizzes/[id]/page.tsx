// /admin/quizzes/[id] — edit, publish/close, duplicate, export, delete.

import Link from "next/link";
import { notFound } from "next/navigation";
import { serverFetch } from "@/lib/api";
import { requireAdminPage } from "@/lib/quiz/admin-gate";
import { Badge, PageHeader } from "@/components/ui/Card";
import { EditorClient } from "../EditorClient";
import { QuizActions } from "./QuizActions";
import { KIND_LABEL, type QuizRecord } from "@/lib/quiz/quiz";

export const dynamic = "force-dynamic";

export default async function EditQuizPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const token = await requireAdminPage();
  let record: QuizRecord;
  try {
    record = await serverFetch<QuizRecord>(`/api/admin/quizzes/${encodeURIComponent(id)}`, token);
  } catch {
    notFound();
  }
  return (
    <main className="quiz-page aq-page">
      <p className="aq-crumb"><Link href="/admin/quizzes">← All quizzes</Link></p>
      <PageHeader
        eyebrow={`${KIND_LABEL[record.kind]} · v${record.version}`}
        title={record.title}
        description={`${record.questions.length} questions · ${record.attempts} attempts`}
        actions={<Badge tone={record.status === "published" ? "green" : record.status === "closed" ? "warm" : "neutral"}>{record.status}</Badge>}
      />
      <QuizActions id={record.id} status={record.status} attempts={record.attempts} title={record.title} />
      <EditorClient key={`${record.id}-${record.updated_at}`} record={record} />
    </main>
  );
}
