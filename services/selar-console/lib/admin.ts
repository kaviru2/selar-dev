// lib/admin.ts — types and helpers for the /admin dashboard.
// Pure functions only (shared by server and client components). Data access
// lives in lib/admin-server.ts, which is server-only.

export interface AdminFilters {
  from: string;
  to: string;
  group: string;
}

export interface DecisionCounts {
  total: number;
  keep: number;
  change: number;
  reject: number;
  retract: number;
  undo: number;
}

export interface DailyPoint {
  day: string;
  active_users: number;
  events: number;
  decisions: number;
  reading_ms: number;
  chat_questions: number;
}

export interface AdminOverview {
  from: string;
  to: string;
  group: string;
  total_users: number;
  consented_users: number;
  admin_users: number;
  active_users: number;
  active_users_last_1d: number;
  active_users_last_7d: number;
  documents: number;
  documents_added: number;
  suggestions_shown: number;
  decisions: DecisionCounts;
  reading_ms: number;
  reading_sessions: number;
  chat_questions: number;
  chat_users: number;
  quiz_submissions: number;
  daily: DailyPoint[] | null;
  groups: { label: string; users: number }[] | null;
  anonymous_counts: Record<string, number> | null;
}

export interface AdminUserRow {
  id: string;
  email: string;
  role: "user" | "admin";
  group_label: string;
  created_at: string;
  consented_at: string | null;
  consent_version: string | null;
  last_seen_at: string | null;
  active_days: number;
  events: number;
  documents: number;
  reading_ms: number;
  suggestions_shown: number;
  decisions: number;
  keep: number;
  change: number;
  reject: number;
  chat_questions: number;
  quiz_attempts: number;
  quiz_avg_pct: number | null;
}

export interface TimelineEvent {
  id: number;
  event: string;
  source: "server" | "client";
  props: Record<string, unknown>;
  occurred_at: string;
}

const DAY = /^\d{4}-\d{2}-\d{2}$/;

export function isAdmin(user: { role?: string } | null | undefined): boolean {
  return user?.role === "admin";
}

type Params = Record<string, string | string[] | undefined>;

function first(v: string | string[] | undefined): string {
  return (Array.isArray(v) ? v[0] : v) ?? "";
}

/** Validates search params so only well-formed filters reach the API. */
export function parseAdminFilters(params: Params): AdminFilters {
  const from = first(params.from);
  const to = first(params.to);
  const group = first(params.group).trim();
  return {
    from: DAY.test(from) ? from : "",
    to: DAY.test(to) ? to : "",
    group: group.length <= 64 ? group : "",
  };
}

export function adminQuery(f: AdminFilters, extra: Record<string, string> = {}): string {
  const q = new URLSearchParams();
  if (f.from) q.set("from", f.from);
  if (f.to) q.set("to", f.to);
  if (f.group) q.set("group", f.group);
  for (const [k, v] of Object.entries(extra)) q.set(k, v);
  const s = q.toString();
  return s ? `?${s}` : "";
}

export function exportHref(kind: "users" | "events", f: AdminFilters, includeEmail: boolean): string {
  return `/api/admin/export/${kind}.csv${adminQuery(f, includeEmail ? { include_email: "true" } : {})}`;
}

export function formatDuration(ms: number): string {
  if (!ms || ms <= 0) return "0m";
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 1) return "<1m";
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

export function pct(part: number, whole: number): string {
  if (!whole) return "—";
  return `${Math.round((part / whole) * 100)}%`;
}

export function formatDay(iso: string | null): string {
  if (!iso) return "—";
  return iso.slice(0, 10);
}
