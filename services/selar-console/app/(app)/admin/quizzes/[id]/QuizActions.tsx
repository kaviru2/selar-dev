"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Button } from "@/components/ui/Button";
import { downloadBlob, quizApi } from "@/lib/quiz/client";
import type { QuizStatus } from "@/lib/quiz/quiz";

export function QuizActions({ id, status, attempts, title }: { id: string; status: QuizStatus; attempts: number; title: string }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true); setError(null);
    try { await fn(); } catch (e) { setError(e instanceof Error ? e.message : "Action failed"); } finally { setBusy(false); }
  };
  const setStatus = (next: QuizStatus, confirmText?: string) => run(async () => {
    if (confirmText && !window.confirm(confirmText)) return;
    await quizApi(`/api/admin/quizzes/${id}/status`, { method: "POST", body: JSON.stringify({ status: next }) });
    router.refresh();
  });

  return (
    <div className="aq-actions">
      {status !== "published" && (
        <Button variant="primary" disabled={busy} onClick={() => setStatus("published", status === "closed" ? "Reopen this quiz to learners?" : "Publish now? Learners in the audience will see it when its window opens.")}>
          {status === "closed" ? "Reopen" : "Publish"}
        </Button>
      )}
      {status === "published" && (
        <Button variant="warm" disabled={busy} onClick={() => setStatus("closed", "Close this quiz? Open attempts are submitted with what has been saved.")}>Close</Button>
      )}
      {status === "published" && attempts === 0 && (
        <Button variant="ghost" disabled={busy} onClick={() => setStatus("draft")}>Back to draft</Button>
      )}
      <Button variant="secondary" disabled={busy} onClick={() => run(async () => {
        const res = await quizApi<{ id: string }>(`/api/admin/quizzes/${id}/duplicate`, { method: "POST" });
        router.push(`/admin/quizzes/${res.id}`);
      })}>Duplicate</Button>
      <Button variant="secondary" disabled={busy} onClick={() => run(async () => {
        const yaml = await quizApi<string>(`/api/admin/quizzes/${id}/export`);
        downloadBlob(`${title.replace(/[^\w-]+/g, "-").toLowerCase() || "quiz"}.yaml`, yaml, "application/yaml");
      })}>Export YAML</Button>
      <Link className="ui-btn" href={`/admin/quizzes/${id}/results`}>Results{attempts ? ` (${attempts})` : ""}</Link>
      {attempts === 0 && (
        <Button variant="ghost" disabled={busy} onClick={() => run(async () => {
          if (!window.confirm("Delete this quiz? This cannot be undone.")) return;
          await quizApi(`/api/admin/quizzes/${id}`, { method: "DELETE" });
          router.push("/admin/quizzes");
        })}>Delete</Button>
      )}
      {error && <p role="alert" className="quiz-error">{error}</p>}
    </div>
  );
}
