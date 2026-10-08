// /admin/quiz-attempts/[id] — blind manual grading of one attempt.

import Link from "next/link";
import { notFound } from "next/navigation";
import { serverFetch } from "@/lib/api";
import { requireAdminPage } from "@/lib/quiz/admin-gate";
import { PageHeader } from "@/components/ui/Card";
import { GradeAttempt, type AdminAttemptDetail } from "@/components/admin-quiz/GradeAttempt";

export const dynamic = "force-dynamic";

export default async function GradePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const token = await requireAdminPage();
  let detail: AdminAttemptDetail;
  try {
    detail = await serverFetch<AdminAttemptDetail>(`/api/admin/quiz-attempts/${encodeURIComponent(id)}`, token);
  } catch {
    notFound();
  }
  return (
    <main className="quiz-page aq-page quiz-page--narrow">
      <p className="aq-crumb"><Link href={`/admin/quizzes/${detail.quiz_id}/results`}>← {detail.quiz_title} results</Link></p>
      <PageHeader eyebrow="Grading (blind)" title={`Attempt ${detail.id.slice(0, 8)}`} description="Learner identity and cohort are hidden while grading." />
      <GradeAttempt detail={detail} />
    </main>
  );
}
