// route.ts — Login API route.
// Proxies login to the Go API and sets the JWT in an httpOnly cookie.
// This keeps the JWT off the client — the browser only sees the cookie.

import { NextResponse } from "next/server";
import { setAuthCookie } from "@/lib/auth";

const API_BASE = process.env.API_INTERNAL_URL || "http://localhost:8080";

export async function POST(request: Request) {
  const body = await request.json();

  const res = await fetch(`${API_BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  const data = await res.json();

  if (!res.ok) {
    return NextResponse.json(data, { status: res.status });
  }

  // Set httpOnly cookie with the JWT
  await setAuthCookie(data.token);

  // Return user data (not the token) to the client
  return NextResponse.json({ user: data.user }, { status: 200 });
}
