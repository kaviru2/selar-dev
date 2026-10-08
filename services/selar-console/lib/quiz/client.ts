// lib/quiz/client.ts — browser fetch helpers for quiz pages (via the /api proxy).

export class QuizApiError extends Error {
  constructor(message: string, public status: number) {
    super(message);
  }
}

export async function quizApi<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && typeof init.body === "string" && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const res = await fetch(path, { ...init, headers });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new QuizApiError(body.error || `Request failed (${res.status})`, res.status);
  }
  const type = res.headers.get("Content-Type") || "";
  return (type.includes("json") ? res.json() : res.text()) as Promise<T>;
}

export function downloadBlob(filename: string, content: string, type: string) {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
