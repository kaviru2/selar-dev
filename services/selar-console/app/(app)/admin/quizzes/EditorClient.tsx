"use client";

import { useRouter } from "next/navigation";
import { QuizEditor } from "@/components/admin-quiz/QuizEditor";
import type { QuizRecord } from "@/lib/quiz/quiz";

export function EditorClient({ record }: { record?: QuizRecord }) {
  const router = useRouter();
  return (
    <QuizEditor
      record={record}
      onSaved={(id) => {
        if (!record) router.push(`/admin/quizzes/${id}`);
        else router.refresh();
      }}
    />
  );
}
