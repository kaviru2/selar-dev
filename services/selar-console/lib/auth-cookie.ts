// auth-cookie.ts — Response-level helpers for the selar_token cookie.
// Free of next/headers so it can be used from route handlers that build
// their own NextResponse (and from the proxy if needed).

import type { NextResponse } from "next/server";

export const AUTH_COOKIE_NAME = "selar_token";

/** Expire selar_token on the given response (same path/flags it was set with). */
export function expireAuthCookie(response: NextResponse): NextResponse {
  response.cookies.set(AUTH_COOKIE_NAME, "", {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 0,
  });
  return response;
}
