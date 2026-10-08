// route.ts — Change password.
// Forwards to the Go API, which verifies the current password, bumps the
// account's session version (ending every other session) and returns a
// replacement token for this one. The token goes into the httpOnly cookie and
// never reaches client JavaScript.

import { NextResponse } from "next/server";
import { getAuthToken, setAuthCookie } from "@/lib/auth";

const API_BASE = process.env.API_INTERNAL_URL || "http://localhost:8080";

export async function POST(request: Request) {
  const token = await getAuthToken();
  if (!token) return NextResponse.json({ error: "unauthorized" }, { status: 401 });

  const res = await fetch(`${API_BASE}/api/users/me/password`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: await request.text(),
    cache: "no-store",
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) return NextResponse.json(data, { status: res.status });

  if (typeof data.token === "string") await setAuthCookie(data.token);
  return NextResponse.json({ other_sessions_ended: Boolean(data.other_sessions_ended) });
}
