import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeToggle } from "@/components/ui/ThemeToggle";
import { SelarProvider } from "./context";
import {
  THEME_INIT_SCRIPT,
  THEME_STORAGE_KEY,
  ThemeProvider,
  resolveTheme,
  useTheme,
} from "./theme";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let systemDark = false;
let listeners: Array<(e: { matches: boolean }) => void> = [];
function mockMatchMedia() {
  window.matchMedia = vi.fn((query: string) => ({
    matches: query.includes("dark") ? systemDark : false,
    media: query,
    addEventListener: (_: string, cb: (e: { matches: boolean }) => void) => listeners.push(cb),
    removeEventListener: (_: string, cb: (e: { matches: boolean }) => void) => {
      listeners = listeners.filter((l) => l !== cb);
    },
  })) as unknown as typeof window.matchMedia;
}

// Node's experimental global localStorage can shadow jsdom's; use a plain stub.
function installStorage() {
  const data = new Map<string, string>();
  const store = {
    getItem: (k: string) => (data.has(k) ? data.get(k)! : null),
    setItem: (k: string, v: string) => void data.set(k, String(v)),
    removeItem: (k: string) => void data.delete(k),
    clear: () => data.clear(),
    key: (i: number) => Array.from(data.keys())[i] ?? null,
    get length() { return data.size; },
  };
  Object.defineProperty(window, "localStorage", { configurable: true, value: store });
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: store });
}

let container: HTMLDivElement;
let root: Root;
const html = () => document.documentElement;

beforeEach(() => {
  systemDark = false;
  listeners = [];
  mockMatchMedia();
  installStorage();
  html().removeAttribute("data-theme");
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
});

function Probe() {
  const { preference, resolvedTheme } = useTheme();
  return <output data-testid="probe">{`${preference}:${resolvedTheme}`}</output>;
}
const probe = () => container.querySelector("[data-testid=probe]")!.textContent;
const radio = (label: string) => container.querySelector(`[role=radio][aria-label="${label}"]`) as HTMLButtonElement;

describe("resolveTheme", () => {
  it("follows the system only for the system preference", () => {
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
    expect(resolveTheme("light", true)).toBe("light");
    expect(resolveTheme("dark", false)).toBe("dark");
  });
});

describe("THEME_INIT_SCRIPT (no flash on load)", () => {
  it("sets data-theme=dark before hydration from storage", () => {
    localStorage.setItem(THEME_STORAGE_KEY, "dark");
    new Function(THEME_INIT_SCRIPT)();
    expect(html().getAttribute("data-theme")).toBe("dark");
  });
  it("follows the system when nothing is stored", () => {
    systemDark = true;
    new Function(THEME_INIT_SCRIPT)();
    expect(html().getAttribute("data-theme")).toBe("dark");
  });
  it("stays light when the user chose light on a dark system", () => {
    systemDark = true;
    localStorage.setItem(THEME_STORAGE_KEY, "light");
    new Function(THEME_INIT_SCRIPT)();
    expect(html().hasAttribute("data-theme")).toBe(false);
  });
});

describe("ThemeProvider + ThemeToggle", () => {
  it("toggles, persists to the device and applies to <html>", async () => {
    await act(async () => root.render(<ThemeProvider><ThemeToggle /><Probe /></ThemeProvider>));
    expect(probe()).toBe("system:light");
    expect(radio("Match device").getAttribute("aria-checked")).toBe("true");

    await act(async () => radio("Dark").click());
    expect(probe()).toBe("dark:dark");
    expect(html().getAttribute("data-theme")).toBe("dark");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");

    await act(async () => radio("Light").click());
    expect(html().hasAttribute("data-theme")).toBe(false);
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("light");
  });

  it("supports arrow keys inside the radio group", async () => {
    await act(async () => root.render(<ThemeProvider><ThemeToggle /><Probe /></ThemeProvider>));
    const group = container.querySelector("[role=radiogroup]")!;
    await act(async () => group.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })));
    expect(probe()).toBe("light:light");
    await act(async () => group.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })));
    expect(probe()).toBe("dark:dark");
  });

  it("reacts live to the OS setting while on system", async () => {
    await act(async () => root.render(<ThemeProvider><Probe /></ThemeProvider>));
    await act(async () => listeners.forEach((l) => l({ matches: true })));
    expect(probe()).toBe("system:dark");
  });
});

describe("account persistence via SelarProvider", () => {
  const user = (preferences: Record<string, unknown>) => ({
    id: "u1", email: "a@b.c", cohort: "control" as const, drive_connected: false, preferences,
  });

  it("applies the account's saved scheme and saves changes without wiping other preferences", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("{}"));
    await act(async () =>
      root.render(
        <ThemeProvider>
          <SelarProvider initialUser={user({ colorScheme: "dark", density: "spacious" })}>
            <ThemeToggle />
            <Probe />
          </SelarProvider>
        </ThemeProvider>
      )
    );
    expect(probe()).toBe("dark:dark");
    expect(fetchSpy).not.toHaveBeenCalled(); // applying the saved value is not a new save

    await act(async () => radio("Light").click());
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const body = JSON.parse(String(fetchSpy.mock.calls[0][1]?.body));
    expect(body).toEqual({ colorScheme: "light", density: "spacious" });
  });

  it("migrates the old theme: 'dark' preference", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("{}"));
    await act(async () =>
      root.render(
        <ThemeProvider>
          <SelarProvider initialUser={user({ theme: "dark" })}>
            <Probe />
          </SelarProvider>
        </ThemeProvider>
      )
    );
    expect(probe()).toBe("dark:dark");
  });
});
