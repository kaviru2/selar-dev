// google-oauth.ts — "Sign in with Google" helpers (authorization-code flow
// with PKCE, state and nonce). The console only starts the flow and checks
// the state; the Go API exchanges the code with the client secret and
// verifies the ID token (signature, aud, iss, exp, nonce, email_verified).
//
// Feature flag: everything is inert unless NEXT_PUBLIC_GOOGLE_CLIENT_ID is
// set (it is inlined at build time, so the button and routes appear only in
// builds made after the variable exists).

export const GOOGLE_AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth";
export const GOOGLE_CALLBACK_PATH = "/api/auth/google/callback";
export const GOOGLE_FLOW_COOKIE = "selar_google_oauth";
export const GOOGLE_FLOW_MAX_AGE = 10 * 60; // seconds to finish signing in at Google

/**
 * Data minimisation: only "openid email". The profile scope (name, picture)
 * is not requested; SELAR stores the verified email and Google's account id.
 */
export const GOOGLE_SIGN_IN_SCOPES = ["openid", "email"] as const;

/** The public OAuth client id, or null when Google sign-in is disabled. */
export function googleClientId(): string | null {
  const id = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID?.trim();
  return id ? id : null;
}

export function googleSignInEnabled(): boolean {
  return googleClientId() !== null;
}

/** Login/register error codes the callback can send back (?google_error=). */
export const GOOGLE_ERROR_MESSAGES: Record<string, string> = {
  cancelled: "Google sign-in was cancelled.",
  expired: "That Google sign-in took too long or was started in another tab. Please try again.",
  unverified: "Your Google account's email address isn't verified, so it can't be used to sign in.",
  linked_elsewhere:
    "This email's SELAR account is linked to a different Google account. Sign in with your password instead.",
  failed: "Google sign-in didn't work. Please try again, or use your email and password.",
};

export function googleErrorMessage(code: string | null | undefined): string {
  if (!code) return "";
  return GOOGLE_ERROR_MESSAGES[code] ?? GOOGLE_ERROR_MESSAGES.failed;
}

function base64url(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function randomToken(byteLength = 32): string {
  const bytes = new Uint8Array(byteLength);
  crypto.getRandomValues(bytes);
  return base64url(bytes);
}

/** RFC 7636 S256 code challenge for a verifier. */
export async function pkceChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return base64url(new Uint8Array(digest));
}

export interface GoogleFlow {
  state: string;
  nonce: string;
  verifier: string;
  /** Validated same-origin destination after sign-in. */
  from: string;
}

export function newGoogleFlow(from: string): GoogleFlow {
  // 32 random bytes -> 43 base64url chars: within PKCE's 43..128 range.
  return { state: randomToken(), nonce: randomToken(), verifier: randomToken(), from };
}

export function encodeFlow(flow: GoogleFlow): string {
  return base64url(new TextEncoder().encode(JSON.stringify(flow)));
}

export function decodeFlow(raw: string | undefined | null): GoogleFlow | null {
  if (!raw) return null;
  try {
    const b64 = raw.replace(/-/g, "+").replace(/_/g, "/");
    const json = new TextDecoder().decode(Uint8Array.from(atob(b64), (c) => c.charCodeAt(0)));
    const v = JSON.parse(json) as Partial<GoogleFlow>;
    if (typeof v.state !== "string" || typeof v.nonce !== "string" || typeof v.verifier !== "string") return null;
    if (!v.state || !v.nonce || v.verifier.length < 43) return null;
    return { state: v.state, nonce: v.nonce, verifier: v.verifier, from: typeof v.from === "string" ? v.from : "" };
  } catch {
    return null;
  }
}

/** Constant-time string comparison (length is not secret here). */
export function safeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

/** The redirect URI registered on the OAuth client for this origin. */
export function googleRedirectUri(origin: string): string {
  return `${origin}${GOOGLE_CALLBACK_PATH}`;
}

export async function googleAuthorizationUrl(opts: {
  clientId: string;
  redirectUri: string;
  flow: GoogleFlow;
}): Promise<URL> {
  const url = new URL(GOOGLE_AUTH_URL);
  url.searchParams.set("client_id", opts.clientId);
  url.searchParams.set("redirect_uri", opts.redirectUri);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("scope", GOOGLE_SIGN_IN_SCOPES.join(" "));
  url.searchParams.set("state", opts.flow.state);
  url.searchParams.set("nonce", opts.flow.nonce);
  url.searchParams.set("code_challenge", await pkceChallenge(opts.flow.verifier));
  url.searchParams.set("code_challenge_method", "S256");
  url.searchParams.set("prompt", "select_account");
  return url;
}
