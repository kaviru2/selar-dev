// route.ts — Logout API route.
// Clears the auth cookie and returns success.

import { NextResponse } from "next/server";
import { clearAuthCookie } from "@/lib/auth";

export async function POST() {
  await clearAuthCookie();
  return NextResponse.json({ status: "ok" });
}
