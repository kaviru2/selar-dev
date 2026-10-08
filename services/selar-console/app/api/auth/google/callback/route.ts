// route.ts — Google redirects here after the user picks an account.
// Checks the state against the flow cookie (one use: the cookie is always
// cleared), then asks the Go API to exchange the code with the PKCE verifier
// and verify the ID token. On success it sets the same selar_token cookie as
// password login and continues to the validated from= destination.

import { NextResponse, type NextRequest } from "next/server";
import { AUTH_COOKIE_NAME } from "@/lib/auth-cookie";
import {
  GOOGLE_FLOW_COOKIE,
  decodeFlow,
  googleClientId,
  googleRedirectUri,
  safeEqual,
} from "@/lib/google-oauth";
import { safeRedirectPath } from "@/lib/safe-redirect";

const API_BASE = process.env.API_INTERNAL_URL || "http://localhost:8080";
const SESSION_MAX_AGE = 7 * 24 * 60 * 60; // matches lib/auth.ts

function finish(request: NextRequest, path: string, error?: string) {
  const url = new URL(path, request.url);
  if (error) url.searchParams.set("google_error", error);
  const response = NextResponse.redirect(url, 303);
  response.headers.set("Cache-Control", "no-store");
  response.cookies.set(GOOGLE_FLOW_COOKIE, "", { httpOnly: true, path: "/api/auth/google", maxAge: 0 });
  return response;
}

export async function GET(request: NextRequest) {
  if (!googleClientId()) {
    return NextResponse.json({ error: "Google sign-in is not enabled" }, { status: 404 });
  }
  const params = request.nextUrl.searchParams;
  const flow = decodeFlow(request.cookies.get(GOOGLE_FLOW_COOKIE)?.value);
  const state = params.get("state") ?? "";

  if (!flow || !state || !safeEqual(state, flow.state)) {
    return finish(request, "/login", "expired");
  }
  if (params.get("error")) {
    return finish(request, "/login", params.get("error") === "access_denied" ? "cancelled" : "failed");
  }
  const code = params.get("code");
  if (!code) return finish(request, "/login", "failed");

  let res: Response;
  try {
    res = await fetch(`${API_BASE}/auth/google`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      cache: "no-store",
      body: JSON.stringify({
        code,
        code_verifier: flow.verifier,
        nonce: flow.nonce,
        redirect_uri: googleRedirectUri(request.nextUrl.origin),
      }),
    });
  } catch {
    return finish(request, "/login", "failed");
  }
  const data = (await res.json().catch(() => ({}))) as { token?: string };
  if (!res.ok || !data.token) {
    const reason = res.status === 403 ? "unverified" : res.status === 409 ? "linked_elsewhere" : "failed";
    return finish(request, "/login", reason);
  }

  const response = finish(request, safeRedirectPath(flow.from));
  response.cookies.set(AUTH_COOKIE_NAME, data.token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: SESSION_MAX_AGE,
  });
  return response;
}
