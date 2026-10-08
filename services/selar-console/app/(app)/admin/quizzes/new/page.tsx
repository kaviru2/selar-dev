import Link from "next/link";
import { requireAdminPage } from "@/lib/quiz/admin-gate";
import { PageHeader } from "@/components/ui/Card";
import { EditorClient } from "../EditorClient";

export default async function NewQuizPage() {
  await requireAdminPage();
  return (
    <main className="quiz-page aq-page">
      <p className="aq-crumb"><Link href="/admin/quizzes">← All quizzes</Link></p>
      <PageHeader eyebrow="Admin" title="New quiz" description="Saved as a draft. Publish it from the quiz page when it is ready." />
      <EditorClient />
    </main>
  );
}
