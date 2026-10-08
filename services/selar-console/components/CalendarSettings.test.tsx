import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CalendarSettings, formatWindow, type CalendarSettingsView } from "./CalendarSettings";

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.unstubAllGlobals(); });

const ok = (body: CalendarSettingsView) => ({ ok: true, json: async () => body });
const enabledView: CalendarSettingsView = {
  available: true, enabled: true,
  feed_url: "https://api.test/calendar/TOKEN.ics",
  google_subscribe_url: "https://calendar.google.com/calendar/r?cid=webcal%3A%2F%2Fapi.test%2Fcalendar%2FTOKEN.ics",
  events: [{ summary: "SELAR: Initial test window", start: "2026-10-12T03:30:00Z", end: "2026-10-12T12:00:00Z", google_url: "https://calendar.google.com/calendar/render?action=TEMPLATE" }],
};

describe("CalendarSettings", () => {
  it("explains that the research team has not switched it on, with no toggle", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(ok({ available: false, enabled: false, events: [] })));
    await act(async () => root.render(<CalendarSettings />));
    expect(container.textContent).toMatch(/Not switched on for this study/);
    expect(container.querySelector('[aria-label="Quiz calendar"]')).toBeNull();
  });

  it("turns the calendar on and shows the private link and windows", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(ok({ available: true, enabled: false, events: [] }))
      .mockResolvedValueOnce(ok(enabledView));
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<CalendarSettings />));
    const on = Array.from(container.querySelectorAll("button")).find((b) => b.textContent === "On") as HTMLButtonElement;
    await act(async () => on.click());
    expect(fetchMock).toHaveBeenLastCalledWith("/api/users/me/calendar", expect.objectContaining({ method: "PUT", body: JSON.stringify({ enabled: true }) }));
    expect((container.querySelector('input[aria-label="Private calendar link"]') as HTMLInputElement).value).toBe(enabledView.feed_url);
    expect(container.textContent).toContain("Initial test window");
    expect(container.querySelector('a[href^="https://calendar.google.com/calendar/r?cid="]')).toBeTruthy();
  });

  it("formats windows in the viewer's time zone (Asia/Colombo)", () => {
    const text = formatWindow(enabledView.events[0], "Asia/Colombo");
    expect(text).toMatch(/9:00/);
    expect(text).toMatch(/5:30/);
  });
});
