// lib/analytics.ts — first-party, consent-gated usage analytics for the console.
//
// Events go only to SELAR's own API (/api/analytics/events → Postgres). There
// are no third-party scripts, no cookies and no local identifiers: the API
// attributes events from the existing httpOnly session.
//
// Usage from any client component:
//
//   import { track } from "@/lib/analytics";
//   track("compare_viewed", { kind: "mental_link", link_id: link.id });
//
// track() is a no-op until <AnalyticsProvider> enables it for a user who has
// opted in, so callers never need to check consent themselves. Never pass
// free text (questions, explanations, passages): send lengths or ids instead.
// The server re-validates everything against the allow-list in
// services/selar-api/internal/analytics and drops anything else.
//
// Server-side facts (decisions, uploads, chat questions, quiz scores) are
// recorded by the Go API itself via h.Track — do not duplicate them here.

export const CLIENT_EVENTS = [
  "app_session_started",
  "app_session_ended",
  "page_viewed",
  "reader_page_viewed",
  "suggestion_shown",
  "suggestion_opened",
  "explain_submitted",
  "compare_viewed",
] as const;

export type ClientEvent = (typeof CLIENT_EVENTS)[number];
export type SuggestionKind = "mental_link" | "passage_link" | "concept" | "concept_edge";

/** Typed properties per client event (mirrors docs/ANALYTICS.md). */
export interface ClientEventProps {
  app_session_started: { route?: string };
  app_session_ended: { duration_ms: number };
  page_viewed: { route: string };
  reader_page_viewed: { document_id?: string; page: number; dwell_ms: number };
  suggestion_shown: { kind: SuggestionKind; link_id?: string; position?: number };
  suggestion_opened: { kind: SuggestionKind; link_id?: string };
  explain_submitted: { link_id?: string; length: number; recall_length?: number; recall_on?: boolean };
  compare_viewed: { kind: SuggestionKind; link_id?: string };
}

export const ANALYTICS_ENDPOINT = "/api/analytics/events";
export const MAX_DWELL_MS = 30 * 60 * 1000;

interface QueuedEvent {
  event: ClientEvent;
  props: Record<string, unknown>;
  at: string;
}

export interface AnalyticsTransport {
  fetch: (url: string, init: RequestInit) => Promise<Response>;
  sendBeacon?: (url: string, body: Blob | string) => boolean;
}

export interface AnalyticsClientOptions {
  transport?: AnalyticsTransport;
  flushMs?: number;
  maxBatch?: number;
  maxQueue?: number;
  now?: () => Date;
}

export interface AnalyticsClient {
  track<E extends ClientEvent>(event: E, props: ClientEventProps[E]): void;
  setEnabled(enabled: boolean): void;
  isEnabled(): boolean;
  flush(): Promise<void>;
  flushOnHide(): void;
}

const ALLOWED = new Set<string>(CLIENT_EVENTS);

function cleanRoute(route: unknown): string | undefined {
  if (typeof route !== "string") return undefined;
  const path = route.split(/[?#]/)[0];
  return path.startsWith("/") && !path.startsWith("//") ? path.slice(0, 120) : undefined;
}

function defaultTransport(): AnalyticsTransport | undefined {
  if (typeof window === "undefined") return undefined;
  return {
    fetch: (url, init) => window.fetch(url, init),
    sendBeacon: typeof navigator !== "undefined" && navigator.sendBeacon ? navigator.sendBeacon.bind(navigator) : undefined,
  };
}

export function createAnalyticsClient(options: AnalyticsClientOptions = {}): AnalyticsClient {
  const flushMs = options.flushMs ?? 10_000;
  const maxBatch = options.maxBatch ?? 50;
  const maxQueue = options.maxQueue ?? 200;
  const now = options.now ?? (() => new Date());
  let transport = options.transport;
  let enabled = false;
  let queue: QueuedEvent[] = [];
  let timer: ReturnType<typeof setTimeout> | null = null;

  const take = () => {
    const batch = queue.slice(0, maxBatch);
    queue = queue.slice(maxBatch);
    return batch;
  };
  const schedule = () => {
    if (timer || typeof setTimeout === "undefined") return;
    timer = setTimeout(() => {
      timer = null;
      void client.flush();
    }, flushMs);
  };

  const client: AnalyticsClient = {
    track(event, props) {
      if (!enabled || !ALLOWED.has(event)) return;
      const clean: Record<string, unknown> = { ...props };
      if ("route" in clean) {
        const route = cleanRoute(clean.route);
        if (route) clean.route = route;
        else delete clean.route;
      }
      if (queue.length >= maxQueue) return;
      queue.push({ event, props: clean, at: now().toISOString() });
      if (queue.length >= maxBatch) void client.flush();
      else schedule();
    },
    setEnabled(next) {
      enabled = next;
      if (!next) queue = [];
    },
    isEnabled: () => enabled,
    async flush() {
      transport ??= defaultTransport();
      if (!enabled || !transport) {
        queue = [];
        return;
      }
      while (queue.length > 0) {
        const batch = take();
        try {
          await transport.fetch(ANALYTICS_ENDPOINT, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ events: batch }),
            keepalive: true,
            credentials: "same-origin",
          });
        } catch {
          // Best effort: analytics must never disturb the user.
        }
      }
    },
    flushOnHide() {
      transport ??= defaultTransport();
      if (!enabled || !transport || queue.length === 0) return;
      while (queue.length > 0) {
        const body = JSON.stringify({ events: take() });
        const sent = transport.sendBeacon?.(ANALYTICS_ENDPOINT, new Blob([body], { type: "application/json" }));
        if (!sent) {
          void transport
            .fetch(ANALYTICS_ENDPOINT, { method: "POST", headers: { "Content-Type": "application/json" }, body, keepalive: true })
            .catch(() => undefined);
        }
      }
    },
  };
  return client;
}

/** The shared browser client used by track(). */
export const analytics = createAnalyticsClient();

/** Record a UI-only analytics event. No-op without consent. */
export function track<E extends ClientEvent>(event: E, props: ClientEventProps[E]): void {
  analytics.track(event, props);
}

export interface DwellTracker {
  enter(page: number): void;
  pause(): void;
  resume(): void;
  stop(): void;
}

/**
 * Measures visible time per reader page. It reports one value per page when
 * the reader leaves it ("throttled" to page changes, never per scroll), skips
 * glances shorter than minMs and caps idle stretches at maxMs.
 */
export function createDwellTracker(
  emit: (page: number, dwellMs: number) => void,
  now: () => number = () => Date.now(),
  { minMs = 1000, maxMs = MAX_DWELL_MS }: { minMs?: number; maxMs?: number } = {},
): DwellTracker {
  let page: number | null = null;
  let accumulated = 0;
  let since: number | null = null;

  const close = () => {
    if (page === null) return;
    const total = accumulated + (since === null ? 0 : now() - since);
    if (total >= minMs) emit(page, Math.min(Math.round(total), maxMs));
    page = null;
    accumulated = 0;
    since = null;
  };
  return {
    enter(next) {
      if (next === page) return;
      close();
      if (Number.isInteger(next) && next > 0) {
        page = next;
        since = now();
      }
    },
    pause() {
      if (page !== null && since !== null) {
        accumulated += now() - since;
        since = null;
      }
    },
    resume() {
      if (page !== null && since === null) since = now();
    },
    stop: close,
  };
}
