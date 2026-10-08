// settings.ts — Client helpers for the Settings page and settings consumers.
//
// Server source of truth: GET/PATCH /api/users/me/settings (selar-api
// internal/settings). Keys:
//   colorScheme               "system" | "light" | "dark"   (lib/theme.tsx)
//   theme                     "paper" | "warm" | "sage"     (light paper tint)
//   reader                    { defaultZoom, rememberPosition, showThumbnails, highlightColor }
//                             — the Reader reads this object (same shape as
//                             lib/reader/position.ts readerPreferences()).
//   suggestions.show_on_open  boolean; may be locked per study cohort.

export type ZoomMode = "fit-width" | "fit-page" | number;
export const HIGHLIGHT_COLORS = ["yellow", "green", "blue", "pink", "purple"] as const;
export type HighlightColor = (typeof HIGHLIGHT_COLORS)[number];

export interface ReaderSettings {
  defaultZoom: ZoomMode;
  rememberPosition: boolean;
  showThumbnails: boolean;
  highlightColor: HighlightColor;
}

export const DEFAULT_READER: ReaderSettings = {
  defaultZoom: "fit-width",
  rememberPosition: true,
  showThumbnails: false,
  highlightColor: "yellow",
};

export const ZOOM_CHOICES: ZoomMode[] = ["fit-width", "fit-page", 0.8, 1, 1.25, 1.5];

export interface SettingsResponse {
  values: Record<string, unknown>;
  locked: string[];
}

export const SUGGESTIONS_ON_OPEN = "suggestions.show_on_open";
export const DELETE_ACCOUNT_CONFIRMATION = "delete my account";

function validZoom(v: unknown): v is ZoomMode {
  return v === "fit-width" || v === "fit-page" || (typeof v === "number" && v >= 0.25 && v <= 4);
}

/** Effective reader defaults from an account preferences object. */
export function readerSettings(prefs: Record<string, unknown> | null | undefined): ReaderSettings {
  const raw = (prefs?.reader && typeof prefs.reader === "object" ? prefs.reader : {}) as Record<string, unknown>;
  return {
    defaultZoom: validZoom(raw.defaultZoom) ? raw.defaultZoom : DEFAULT_READER.defaultZoom,
    rememberPosition: typeof raw.rememberPosition === "boolean" ? raw.rememberPosition : DEFAULT_READER.rememberPosition,
    showThumbnails: typeof raw.showThumbnails === "boolean" ? raw.showThumbnails : DEFAULT_READER.showThumbnails,
    highlightColor: (HIGHLIGHT_COLORS as readonly string[]).includes(raw.highlightColor as string)
      ? (raw.highlightColor as HighlightColor)
      : DEFAULT_READER.highlightColor,
  };
}

export function zoomLabel(z: ZoomMode): string {
  if (z === "fit-width") return "Fit width";
  if (z === "fit-page") return "Fit page";
  return `${Math.round(z * 100)}%`;
}

/**
 * Starting scale for the current single-page reader, which only supports
 * numeric zoom (60–180%). Fit modes start at 100% until the continuous reader
 * (which understands them natively) lands.
 */
export function initialZoom(z: ZoomMode): number {
  if (typeof z !== "number") return 1;
  return Math.min(1.8, Math.max(0.6, z));
}

/** Whether the Reader shows suggestion highlights when a document opens. */
export function suggestionsOnOpen(prefs: Record<string, unknown> | null | undefined): boolean {
  return prefs?.[SUGGESTIONS_ON_OPEN] !== false;
}

/**
 * Client-side copy of the API's change-password rules (internal/settings
 * CheckPasswordStrength). The API remains the authority; this only gives
 * immediate feedback. Returns a message, or null when acceptable.
 */
export function passwordProblem(password: string, email: string): string | null {
  const chars = Array.from(password);
  if (chars.length < 10) return "Use at least 10 characters.";
  if (new TextEncoder().encode(password).length > 72) return "Use at most 72 bytes (shorter, or fewer accented characters).";
  const letter = chars.some((c) => /\p{L}/u.test(c));
  const other = chars.some((c) => !/\p{L}/u.test(c));
  if (!letter || !other) return "Please mix letters with numbers, spaces or symbols.";
  if (new Set(chars.map((c) => c.toLowerCase())).size < 4) return "That password is too repetitive.";
  if (email && password.toLowerCase().includes(email.toLowerCase())) return "Don't include your email address.";
  return null;
}

async function send<T>(path: string, method: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((data as { error?: string }).error || `Request failed (${res.status})`);
  return data as T;
}

export function loadSettings(): Promise<SettingsResponse> {
  return send<SettingsResponse>("/api/users/me/settings", "GET");
}

export function saveSettings(patch: Record<string, unknown>): Promise<SettingsResponse> {
  return send<SettingsResponse>("/api/users/me/settings", "PATCH", patch);
}

export function saveDisplayName(display_name: string): Promise<{ display_name: string }> {
  return send("/api/users/me/profile", "PATCH", { display_name });
}

export function changeEmail(email: string, current_password: string): Promise<{ email: string }> {
  return send("/api/users/me/email", "POST", { email, current_password });
}

/** Changes the password; the route handler swaps in the new session cookie. */
export function changePassword(current_password: string, new_password: string): Promise<{ other_sessions_ended: boolean }> {
  return send("/api/auth/password", "POST", { current_password, new_password });
}

export function setResearchConsent(granted: boolean): Promise<{ consented_at: string | null }> {
  return send("/api/users/me/research-consent", "PUT", { granted, via: "settings" });
}

/** Deletes the account; the route handler clears the session cookie. */
export function deleteAccount(current_password: string): Promise<{ status: string }> {
  return send("/api/auth/delete-account", "POST", { current_password, confirm: DELETE_ACCOUNT_CONFIRMATION });
}
