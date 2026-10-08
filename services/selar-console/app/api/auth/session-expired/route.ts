// route.ts — Session-expired route (redirect-loop breaker).
// Server Components cannot modify cookies, so when the (app) layout finds
// that the Go API rejects the stored JWT it redirects here. We expire the
// selar_token cookie and send the browser to /login, so the proxy no longer
// bounces the user from /login back into the app.

import { NextResponse, type NextRequest } from "next/server";
import { expireAuthCookie } from "@/lib/auth-cookie";
import { safeRedirectPath } from "@/lib/safe-redirect";

export function GET(request: NextRequest) {
  const loginUrl = new URL("/login", request.url);
  const from = request.nextUrl.searchParams.get("from");
  if (from && safeRedirectPath(from) === from) {
    loginUrl.searchParams.set("from", from);
  }
  const response = NextResponse.redirect(loginUrl, 303);
  response.headers.set("Cache-Control", "no-store");
  return expireAuthCookie(response);
}
