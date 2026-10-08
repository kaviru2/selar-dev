// lib/admin-server.ts — server-only access to the Go admin API.
// requireAdmin() is the server-side gate for every /admin page: it re-checks
// the role with the API (which reads users.role from the database) and calls
// forbidden() otherwise, so hiding the nav item is never the only protection.

import { forbidden, redirect } from "next/navigation";
import { getAuthToken, getSessionStatus, type SessionUser } from "@/lib/auth";
import { adminQuery, isAdmin, type AdminFilters } from "@/lib/admin";

const API_BASE = process.env.API_INTERNAL_URL || "http://localhost:8080";

export async function requireAdmin(): Promise<SessionUser> {
  const session = await getSessionStatus();
  if (session.status === "anonymous") redirect("/login?from=/admin");
  if (session.status === "invalid") redirect("/api/auth/session-expired");
  if (session.status !== "authenticated" || !isAdmin(session.user)) forbidden();
  return session.user;
}

export class AdminApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

/** GET an admin endpoint as the signed-in admin. 403 from the API → forbidden(). */
export async function adminGet<T>(path: string, filters?: AdminFilters, extra?: Record<string, string>): Promise<T> {
  const token = await getAuthToken();
  if (!token) redirect("/login?from=/admin");
  const res = await fetch(`${API_BASE}/api/admin${path}${filters ? adminQuery(filters, extra) : ""}`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (res.status === 401) redirect("/api/auth/session-expired");
  if (res.status === 403) forbidden();
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new AdminApiError(res.status, body.error || `admin API error ${res.status}`);
  }
  return res.json() as Promise<T>;
}
