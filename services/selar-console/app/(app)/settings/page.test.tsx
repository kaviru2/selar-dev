import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SelarProvider } from "@/lib/context";
import { ThemeProvider } from "@/lib/theme";
import SettingsPage from "./page";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let host: HTMLDivElement;
let calls: Array<{ url: string; method: string; body: unknown }>;
let locked: string[];

function respond(url: string, init?: RequestInit) {
  const method = init?.method ?? "GET";
  const body = init?.body ? JSON.parse(String(init.body)) : undefined;
  calls.push({ url, method, body });
  if (url === "/api/users/me/settings") {
    const values = { colorScheme: "system", theme: "paper", reader: { defaultZoom: "fit-width", rememberPosition: true, showThumbnails: false, highlightColor: "yellow", ...(body?.reader ?? {}) }, "suggestions.show_on_open": true };
    return new Response(JSON.stringify({ values, locked }), { status: 200 });
  }
  if (url === "/api/users/me/research-consent") {
    return new Response(JSON.stringify({ consented_at: body.granted ? "2026-10-08T00:00:00Z" : null }), { status: 200 });
  }
  return new Response("{}", { status: 200 });
}

beforeEach(() => {
  calls = [];
  locked = [];
  window.matchMedia = vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => respond(String(url), init));
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.restoreAllMocks();
});

const user = { id: "u1", email: "qa@example.com", display_name: "", cohort: "control" as const, drive_connected: false, preferences: {}, consented_at: null };

async function render() {
  await act(async () =>
    root.render(
      <ThemeProvider>
        <SelarProvider initialUser={user}>
          <SettingsPage />
        </SelarProvider>
      </ThemeProvider>
    )
  );
}

const byText = (text: string) => Array.from(host.querySelectorAll("button, a")).find((el) => el.textContent?.trim() === text) as HTMLElement;
const byLabel = (label: string) => host.querySelector(`[aria-label="${label}"]`) as HTMLInputElement;

function type(el: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  setter.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("Settings page", () => {
  it("has no placeholder controls left", async () => {
    await render();
    expect(host.textContent).not.toMatch(/Unavailable|Google Drive|Density/);
    expect(host.querySelectorAll("button[disabled]").length).toBeGreaterThan(0); // forms start disabled until filled
  });

  it("saves reader defaults as a partial 'reader' patch", async () => {
    await render();
    await act(async () => byText("green").click());
    const patch = calls.find((c) => c.method === "PATCH" && c.url === "/api/users/me/settings");
    expect(patch?.body).toEqual({ reader: { highlightColor: "green" } });
    expect(byText("green").getAttribute("aria-checked")).toBe("true");
  });

  it("disables a setting the study has locked", async () => {
    locked = ["suggestions.show_on_open"];
    await render();
    const box = host.querySelector('input[type="checkbox"][disabled]');
    expect(box).not.toBeNull();
    expect(host.textContent).toContain("Set by your study group");
  });

  it("gives every preference checkbox a name that says what it controls", async () => {
    locked = ["suggestions.show_on_open"];
    await render();
    const names = Array.from(host.querySelectorAll('input[type="checkbox"]')).map((box) => box.getAttribute("aria-label"));
    expect(names).toEqual(["Remember position", "Page thumbnails", "Show suggestions when a document opens"]);
    const lockedBox = byLabel("Show suggestions when a document opens");
    expect(lockedBox.disabled).toBe(true);
    expect(lockedBox.title).toBe("Set by your study group; it cannot be changed here.");
  });

  it("shows the save status in the section that was changed", async () => {
    await render();
    await act(async () => byText("green").click());
    const reader = host.querySelector('section[aria-label="Reader"]')!;
    const suggestions = host.querySelector('section[aria-label="Suggestions"]')!;
    expect(reader.querySelector('[role="status"]')?.textContent).toBe("Saved");
    expect(suggestions.querySelector('[role="status"]')).toBeNull();
    await act(async () => byLabel("Show suggestions when a document opens").click());
    expect(suggestions.querySelector('[role="status"]')?.textContent).toBe("Saved");
    expect(reader.querySelector('[role="status"]')).toBeNull();
  });

  it("only enables Delete account after the exact phrase and a password", async () => {
    await render();
    const del = byText("Delete account") as HTMLButtonElement;
    expect(del.disabled).toBe(true);
    await act(async () => type(byLabel("Current password (to delete account)"), "pw"));
    await act(async () => type(byLabel("Type delete my account to confirm"), "delete"));
    expect(del.disabled).toBe(true);
    await act(async () => type(byLabel("Type delete my account to confirm"), "delete my account"));
    expect(del.disabled).toBe(false);
  });

  it("shows password rule feedback before submitting", async () => {
    await render();
    await act(async () => type(byLabel("New password"), "short"));
    expect(host.textContent).toContain("at least 10");
  });

  it("toggles usage analytics consent through the analytics API", async () => {
    await render();
    await act(async () => byText("Turn on").click());
    const put = calls.find((c) => c.url === "/api/users/me/research-consent");
    expect(put).toMatchObject({ method: "PUT", body: { granted: true, via: "settings" } });
    expect(byText("Turn off")).toBeTruthy();
  });

  it("links the export download", async () => {
    await render();
    expect(byText("Download export").getAttribute("href")).toBe("/api/users/me/export");
  });
});
