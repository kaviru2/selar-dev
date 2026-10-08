import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { analytics } from "@/lib/analytics";
import { AnalyticsProvider } from "./AnalyticsProvider";

vi.mock("next/navigation", () => ({ usePathname: () => "/library" }));

const fetchMock = vi.fn();
let container: HTMLDivElement;
let root: Root;

const settle = () => act(async () => { for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0)); });
const dialog = () => container.querySelector('[role="dialog"]');
const button = (name: RegExp) => Array.from(container.querySelectorAll("button")).find((b) => name.test(b.textContent || "")) as HTMLButtonElement | undefined;
const mount = async (user: { id: string; consented_at: string | null; consent_decided_at: string | null }) => {
  await act(async () => root.render(<AnalyticsProvider user={user}><p>app</p></AnalyticsProvider>));
  await settle();
};

beforeEach(() => {
  fetchMock.mockReset();
  fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
    if (url === "/api/analytics/config") return new Response(JSON.stringify({ enabled: true, consent_version: "prototype-usage-v1" }), { status: 200 });
    if (url === "/api/users/me/research-consent") {
      const granted = JSON.parse(String(init?.body)).granted;
      return new Response(JSON.stringify({ consented_at: granted ? "2026-10-08T00:00:00Z" : null, consent_decided_at: "2026-10-08T00:00:00Z" }), { status: 200 });
    }
    return new Response(null, { status: 202 });
  });
  vi.stubGlobal("fetch", fetchMock);
  analytics.setEnabled(false);
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe("AnalyticsProvider", () => {
  it("keeps analytics off and shows a one-time prompt for undecided users", async () => {
    await mount({ id: "u", consented_at: null, consent_decided_at: null });
    expect(dialog()?.textContent).toMatch(/help improve selar/i);
    expect(analytics.isEnabled()).toBe(false);
  });

  it("enables analytics after an explicit yes and records the choice", async () => {
    await mount({ id: "u", consented_at: null, consent_decided_at: null });
    await act(async () => { button(/yes, record my usage/i)!.click(); });
    await settle();
    const call = fetchMock.mock.calls.find(([url]) => url === "/api/users/me/research-consent");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ granted: true, via: "prompt" });
    expect(analytics.isEnabled()).toBe(true);
    expect(dialog()).toBeNull();
  });

  it("stays off after 'No thanks'", async () => {
    await mount({ id: "u", consented_at: null, consent_decided_at: null });
    await act(async () => { button(/no thanks/i)!.click(); });
    await settle();
    expect(analytics.isEnabled()).toBe(false);
    expect(dialog()).toBeNull();
  });

  it("does not prompt users who already decided, and enables only consenting ones", async () => {
    await mount({ id: "u", consented_at: null, consent_decided_at: "2026-10-01T00:00:00Z" });
    expect(dialog()).toBeNull();
    expect(analytics.isEnabled()).toBe(false);
    act(() => root.unmount());
    root = createRoot(container);
    await mount({ id: "u", consented_at: "2026-10-01T00:00:00Z", consent_decided_at: "2026-10-01T00:00:00Z" });
    expect(analytics.isEnabled()).toBe(true);
  });
});
