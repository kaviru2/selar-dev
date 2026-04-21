// route.ts — Preferences proxy route.
// Forwards preference updates (theme, density) to the Go API
// using the JWT from the httpOnly cookie.

import { NextResponse } from "next/server";
import { getAuthToken } from "@/lib/auth";

const API_BASE = process.env.API_INTERNAL_URL || "http://localhost:8080";

export async function PATCH(request: Request) {
  const token = await getAuthToken();
  if (!token) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const body = await request.json();

  const res = await fetch(`${API_BASE}/api/users/me/preferences`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
  });

  const data = await res.json();
  return NextResponse.json(data, { status: res.status });
}
