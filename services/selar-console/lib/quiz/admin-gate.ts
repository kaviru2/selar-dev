// lib/quiz/admin-gate.ts — server-side check that the signed-in user may use
// /admin/quizzes. The API enforces this on every admin route too; this just
// avoids rendering admin chrome for non-admins.

import { notFound } from "next/navigation";
import { getAuthToken } from "@/lib/auth";
import { serverFetch } from "@/lib/api";

export async function requireAdminPage(): Promise<string> {
  const token = await getAuthToken();
  if (!token) notFound();
  try {
    const res = await serverFetch<{ admin: boolean }>("/api/admin/access", token);
    if (!res.admin) notFound();
  } catch {
    notFound();
  }
  return token;
}
