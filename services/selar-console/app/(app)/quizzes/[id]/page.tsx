"use client";

// /quizzes/[id] — starts (or resumes) an attempt. The API returns the open
// attempt if one exists, so refreshing never burns an attempt.

import { use, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { TakeQuiz } from "@/components/quiz/TakeQuiz";
import { quizApi } from "@/lib/quiz/client";
import type { AttemptView } from "@/lib/quiz/quiz";

export default function TakeQuizPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const [view, setView] = useState<AttemptView | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    quizApi<AttemptView>(`/api/quizzes/${id}/attempts`, { method: "POST" })
      .then((v) => { if (!cancelled) setView(v); })
      .catch((e: Error) => { if (!cancelled) setError(e.message); });
    return () => { cancelled = true; };
  }, [id]);

  const done = useCallback((attemptId: string) => {
    router.replace(`/quizzes/result/${attemptId}`);
    router.refresh();
  }, [router]);

  return (
    <main className="quiz-page quiz-page--narrow">
      {error && (
        <div className="quiz-error-block" role="alert">
          <h1>This quiz cannot be started</h1>
          <p>{error}</p>
          <Link href="/quizzes">Back to quizzes</Link>
        </div>
      )}
      {!error && !view && <p className="quiz-loading" aria-live="polite">Loading quiz…</p>}
      {view && <TakeQuiz initial={view} onDone={done} />}
    </main>
  );
}
