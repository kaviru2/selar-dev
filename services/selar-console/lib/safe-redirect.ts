// safe-redirect.ts — Validates post-login destinations.
// Only same-origin, relative app paths are allowed; anything that could
// leave the origin (//host, backslashes, schemes) or bounce back into the
// auth flow (/, /login, /register, /api/...) falls back to the library.

export const DEFAULT_SIGNED_IN_PATH = "/library";

const ORIGIN_PROBE = "http://selar.invalid";

/** True for /login, /register and their sub-paths (not look-alikes like /loginx). */
export function isAuthPagePath(pathname: string): boolean {
  return ["/login", "/register"].some((p) => pathname === p || pathname.startsWith(`${p}/`));
}

export function safeRedirectPath(from: string | null | undefined): string {
  if (!from || !from.startsWith("/") || from.startsWith("//")) return DEFAULT_SIGNED_IN_PATH;
  // Backslashes are normalised to "/" by browsers; control chars/whitespace can be stripped.
  if (/[\\\s\u0000-\u001f\u007f]/.test(from)) return DEFAULT_SIGNED_IN_PATH;

  let decoded: string;
  try {
    decoded = decodeURIComponent(from);
  } catch {
    return DEFAULT_SIGNED_IN_PATH;
  }
  if (decoded.startsWith("//") || /[\\\s\u0000-\u001f\u007f]/.test(decoded)) return DEFAULT_SIGNED_IN_PATH;

  let url: URL;
  try {
    url = new URL(from, ORIGIN_PROBE);
  } catch {
    return DEFAULT_SIGNED_IN_PATH;
  }
  if (url.origin !== ORIGIN_PROBE) return DEFAULT_SIGNED_IN_PATH;

  const { pathname } = url;
  if (pathname === "/" || isAuthPagePath(pathname) || pathname === "/api" || pathname.startsWith("/api/")) {
    return DEFAULT_SIGNED_IN_PATH;
  }
  return `${pathname}${url.search}${url.hash}`;
}
