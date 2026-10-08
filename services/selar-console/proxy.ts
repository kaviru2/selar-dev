// proxy.ts — Next.js 16 proxy (formerly middleware) for route protection.
// - Signed-out visitors are sent to /login (with from= for deep links).
// - Signed-in visitors (selar_token present) hitting /, /login or /register
//   are sent into the app: to a validated same-origin from=, else /library.
// The cookie is only checked for presence here. When the Go API rejects it,
// the (app) layout and the /api proxy expire it (see /api/auth/session-expired)
// so /login becomes reachable again and no redirect loop can form.

import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";
import { AUTH_COOKIE_NAME } from "@/lib/auth-cookie";
import { isAuthPagePath, safeRedirectPath } from "@/lib/safe-redirect";

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;

  // Auth API routes (login, register, logout, session-expired) are always reachable.
  if (pathname === "/api/auth" || pathname.startsWith("/api/auth/")) {
    return NextResponse.next();
  }

  // Allow static assets and Next.js internals
  if (
    pathname.startsWith("/_next") ||
    pathname.startsWith("/favicon") ||
    pathname.includes(".")
  ) {
    return NextResponse.next();
  }

  const signedIn = Boolean(request.cookies.get(AUTH_COOKIE_NAME)?.value);

  if (pathname === "/" || isAuthPagePath(pathname)) {
    if (signedIn) {
      const from = pathname === "/" ? null : request.nextUrl.searchParams.get("from");
      return NextResponse.redirect(new URL(safeRedirectPath(from), request.url));
    }
    if (pathname === "/") {
      return NextResponse.redirect(new URL("/login", request.url));
    }
    return NextResponse.next();
  }

  if (!signedIn) {
    const loginUrl = new URL("/login", request.url);
    loginUrl.searchParams.set("from", pathname);
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    // Match all routes except static files and API routes
    "/((?!_next/static|_next/image|favicon.ico).*)",
  ],
};
