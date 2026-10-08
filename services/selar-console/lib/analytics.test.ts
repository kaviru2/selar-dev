import { afterEach, describe, expect, it, vi } from "vitest";
import { createAnalyticsClient, createDwellTracker, CLIENT_EVENTS } from "./analytics";

function harness() {
  const sent: Array<{ url: string; body: string; viaBeacon: boolean }> = [];
  const transport = {
    fetch: vi.fn(async (url: string, init: RequestInit) => {
      sent.push({ url, body: String(init.body), viaBeacon: false });
      return new Response(null, { status: 202 });
    }),
    sendBeacon: vi.fn((url: string, body: Blob | string) => {
      sent.push({ url, body: typeof body === "string" ? body : "[blob]", viaBeacon: true });
      return true;
    }),
  };
  return { sent, transport };
}

afterEach(() => vi.useRealTimers());

describe("analytics client", () => {
  it("does nothing until enabled (no consent → no network)", async () => {
    const { sent, transport } = harness();
    const client = createAnalyticsClient({ transport, flushMs: 10 });
    client.track("page_viewed", { route: "/library" });
    await client.flush();
    expect(sent).toHaveLength(0);
  });

  it("batches events into one POST to the first-party endpoint", async () => {
    const { sent, transport } = harness();
    const client = createAnalyticsClient({ transport, flushMs: 10_000 });
    client.setEnabled(true);
    client.track("page_viewed", { route: "/library" });
    client.track("suggestion_shown", { kind: "mental_link", link_id: "6f1c1d0e-8d1c-4e7e-9a1b-111111111111" });
    await client.flush();
    expect(sent).toHaveLength(1);
    expect(sent[0].url).toBe("/api/analytics/events");
    const body = JSON.parse(sent[0].body);
    expect(body.events.map((e: { event: string }) => e.event)).toEqual(["page_viewed", "suggestion_shown"]);
    expect(typeof body.events[0].at).toBe("string");
  });

  it("drops unknown event names and strips query strings from routes", async () => {
    const { sent, transport } = harness();
    const client = createAnalyticsClient({ transport });
    client.setEnabled(true);
    // @ts-expect-error — not an allow-listed event
    client.track("keystroke_log", { text: "secret" });
    client.track("page_viewed", { route: "/reader?doc=abc#p2" });
    await client.flush();
    const body = JSON.parse(sent[0].body);
    expect(body.events).toEqual([expect.objectContaining({ event: "page_viewed", props: { route: "/reader" } })]);
  });

  it("flushes with sendBeacon when the page is hidden", () => {
    const { sent, transport } = harness();
    const client = createAnalyticsClient({ transport });
    client.setEnabled(true);
    client.track("app_session_ended", { duration_ms: 1000 });
    client.flushOnHide();
    expect(sent[0].viaBeacon).toBe(true);
  });

  it("caps the queue so a stuck tab cannot grow without bound", async () => {
    const { sent, transport } = harness();
    const client = createAnalyticsClient({ transport, maxQueue: 5, maxBatch: 50 });
    client.setEnabled(true);
    for (let i = 0; i < 20; i++) client.track("page_viewed", { route: `/p${i}` });
    await client.flush();
    expect(JSON.parse(sent[0].body).events).toHaveLength(5);
  });

  it("clears queued events when disabled (consent withdrawn)", async () => {
    const { sent, transport } = harness();
    const client = createAnalyticsClient({ transport });
    client.setEnabled(true);
    client.track("page_viewed", { route: "/library" });
    client.setEnabled(false);
    await client.flush();
    expect(sent).toHaveLength(0);
  });

  it("exports the same client allow-list as the API", () => {
    expect([...CLIENT_EVENTS].sort()).toEqual([
      "app_session_ended",
      "app_session_started",
      "compare_viewed",
      "explain_submitted",
      "page_viewed",
      "reader_page_viewed",
      "suggestion_opened",
      "suggestion_shown",
    ]);
  });
});

describe("reader dwell tracker", () => {
  it("emits one event per page left, ignoring sub-second glances and capping idle time", () => {
    let now = 0;
    const events: Array<{ page: number; dwell_ms: number }> = [];
    const dwell = createDwellTracker((page, dwellMs) => events.push({ page, dwell_ms: dwellMs }), () => now, { minMs: 1000, maxMs: 30 * 60 * 1000 });
    dwell.enter(1);
    now = 5_000;
    dwell.enter(2); // 5 s on page 1
    now = 5_400;
    dwell.enter(3); // 0.4 s glance on page 2 → ignored
    now = 5_400 + 2 * 60 * 60 * 1000;
    dwell.stop(); // 2 h idle on page 3 → capped
    expect(events).toEqual([
      { page: 1, dwell_ms: 5000 },
      { page: 3, dwell_ms: 30 * 60 * 1000 },
    ]);
  });

  it("pauses while the tab is hidden", () => {
    let now = 0;
    const events: number[] = [];
    const dwell = createDwellTracker((_p, ms) => events.push(ms), () => now);
    dwell.enter(1);
    now = 3_000;
    dwell.pause();
    now = 600_000;
    dwell.resume();
    now = 602_000;
    dwell.stop();
    expect(events).toEqual([5_000]);
  });
});
