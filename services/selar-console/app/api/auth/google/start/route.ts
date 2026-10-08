// route.ts — Starts "Sign in with Google".
// Creates state, nonce and a PKCE verifier, keeps them in a short-lived
// httpOnly cookie scoped to /api/auth/google, and redirects to Google.
// Returns 404 when NEXT_PUBLIC_GOOGLE_CLIENT_ID is unset (feature off).

import { NextResponse, type NextRequest } from "next/server";
import {
  GOOGLE_FLOW_COOKIE,
  GOOGLE_FLOW_MAX_AGE,
  encodeFlow,
  googleAuthorizationUrl,
  googleClientId,
  googleRedirectUri,
  newGoogleFlow,
} from "@/lib/google-oauth";
import { safeRedirectPath } from "@/lib/safe-redirect";

export async function GET(request: NextRequest) {
  const clientId = googleClientId();
  if (!clientId) {
    return NextResponse.json({ error: "Google sign-in is not enabled" }, { status: 404 });
  }
  const flow = newGoogleFlow(safeRedirectPath(request.nextUrl.searchParams.get("from")));
  const url = await googleAuthorizationUrl({
    clientId,
    redirectUri: googleRedirectUri(request.nextUrl.origin),
    flow,
  });
  const response = NextResponse.redirect(url, 307);
  response.headers.set("Cache-Control", "no-store");
  response.cookies.set(GOOGLE_FLOW_COOKIE, encodeFlow(flow), {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    // Lax: the cookie must come back on Google's top-level GET redirect.
    sameSite: "lax",
    path: "/api/auth/google",
    maxAge: GOOGLE_FLOW_MAX_AGE,
  });
  return response;
}
