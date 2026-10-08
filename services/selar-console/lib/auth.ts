// lib/auth.ts — Server-side JWT auth utilities for SELAR console.
// Manages httpOnly cookies for JWT storage. The token never reaches
// client-side JavaScript — all auth state is read server-side.

import { cookies } from "next/headers";

const COOKIE_NAME = "selar_token";
const COOKIE_MAX_AGE = 7 * 24 * 60 * 60; // 7 days

const API_BASE =
  process.env.API_INTERNAL_URL || "http://localhost:8080";

/**
 * Store JWT in an httpOnly cookie after successful login/register.
 */
export async function setAuthCookie(token: string): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.set(COOKIE_NAME, token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: COOKIE_MAX_AGE,
  });
}

/**
 * Read the JWT from the cookie. Returns null if not present.
 */
export async function getAuthToken(): Promise<string | null> {
  const cookieStore = await cookies();
  const cookie = cookieStore.get(COOKIE_NAME);
  return cookie?.value ?? null;
}

/**
 * Clear the auth cookie (logout).
 */
export async function clearAuthCookie(): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.delete(COOKIE_NAME);
}

/**
 * Fetch the current user from the Go API using the stored JWT.
 * Returns null if not authenticated or token is invalid.
 */
export async function getSession(): Promise<SessionUser | null> {
  const token = await getAuthToken();
  if (!token) return null;

  try {
    const res = await fetch(`${API_BASE}/api/users/me`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    });
    if (!res.ok) return null;
    const user = (await res.json()) as SessionUser;
    return user;
  } catch {
    return null;
  }
}

export type SessionStatus =
  | { status: "anonymous" }
  | { status: "authenticated"; user: SessionUser }
  /** The Go API rejected the token (401): the cookie must be cleared. */
  | { status: "invalid" }
  /** The API could not be reached or errored: keep the cookie. */
  | { status: "unavailable" };

/**
 * Like getSession(), but distinguishes a rejected token from an API outage,
 * so callers only force a logout when the token is actually invalid.
 */
export async function getSessionStatus(): Promise<SessionStatus> {
  const token = await getAuthToken();
  if (!token) return { status: "anonymous" };

  try {
    const res = await fetch(`${API_BASE}/api/users/me`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    });
    if (res.status === 401) return { status: "invalid" };
    if (!res.ok) return { status: "unavailable" };
    return { status: "authenticated", user: (await res.json()) as SessionUser };
  } catch {
    return { status: "unavailable" };
  }
}

/** The user shape returned by getSession(). */
export interface SessionUser {
  id: string;
  email: string;
  cohort: "control" | "treatment_auto" | "treatment_hitl";
  drive_connected: boolean;
  preferences: Record<string, unknown>;
  created_at: string;
}
