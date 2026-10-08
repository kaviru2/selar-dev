// route.ts — Delete my account.
// Forwards the password + typed confirmation to the Go API and, once the
// account is gone, clears the session cookie so the browser is signed out.

import { NextResponse } from "next/server";
import { clearAuthCookie, getAuthToken } from "@/lib/auth";

const API_BASE = process.env.API_INTERNAL_URL || "http://localhost:8080";

export async function POST(request: Request) {
  const token = await getAuthToken();
  if (!token) return NextResponse.json({ error: "unauthorized" }, { status: 401 });

  const res = await fetch(`${API_BASE}/api/users/me/delete`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: await request.text(),
    cache: "no-store",
  });
  const data = await res.json().catch(() => ({}));
  if (res.ok) await clearAuthCookie();
  return NextResponse.json(data, { status: res.status });
}
