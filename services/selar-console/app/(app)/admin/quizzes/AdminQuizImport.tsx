"use client";

import { useRouter } from "next/navigation";
import { ImportPanel } from "@/components/admin-quiz/ImportPanel";

export function AdminQuizImport() {
  const router = useRouter();
  return <ImportPanel onImported={(id) => router.push(`/admin/quizzes/${id}`)} />;
}
