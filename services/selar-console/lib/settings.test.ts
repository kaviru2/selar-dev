import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DELETE_ACCOUNT_CONFIRMATION,
  initialZoom,
  passwordProblem,
  readerSettings,
  saveSettings,
  suggestionsOnOpen,
  zoomLabel,
} from "./settings";

afterEach(() => vi.restoreAllMocks());

describe("passwordProblem mirrors the API rules", () => {
  it("accepts a mixed password of 10+ characters", () => {
    expect(passwordProblem("reading list 42", "a@example.com")).toBeNull();
  });
  it.each([
    ["short 1", "at least 10"],
    ["onlyletterslong", "mix letters"],
    ["1234567890", "mix letters"],
    ["aaaaaaaaa1", "repetitive"],
    ["x" + "é".repeat(40), "72 bytes"],
    ["me@example.com 1", "email"],
  ])("rejects %s", (pw, msg) => {
    expect(passwordProblem(pw, "me@example.com")).toContain(msg);
  });
});

describe("readerSettings", () => {
  it("fills defaults and keeps valid stored fields", () => {
    expect(readerSettings(undefined)).toEqual({
      defaultZoom: "fit-width",
      rememberPosition: true,
      showThumbnails: false,
      highlightColor: "yellow",
    });
    expect(readerSettings({ reader: { defaultZoom: 1.25, highlightColor: "green", junk: 1 } })).toMatchObject({
      defaultZoom: 1.25,
      highlightColor: "green",
    });
  });
  it("ignores invalid stored values", () => {
    expect(readerSettings({ reader: { defaultZoom: 9, highlightColor: "#f00" } })).toMatchObject({
      defaultZoom: "fit-width",
      highlightColor: "yellow",
    });
  });
});

describe("zoom helpers", () => {
  it("labels zoom modes", () => {
    expect(zoomLabel("fit-width")).toBe("Fit width");
    expect(zoomLabel(1.25)).toBe("125%");
  });
  it("maps fit modes to 100% for the current reader and clamps numbers", () => {
    expect(initialZoom("fit-width")).toBe(1);
    expect(initialZoom(1.5)).toBe(1.5);
    expect(initialZoom(3)).toBe(1.8);
  });
});

describe("suggestionsOnOpen", () => {
  it("defaults to on and respects false", () => {
    expect(suggestionsOnOpen(undefined)).toBe(true);
    expect(suggestionsOnOpen({ "suggestions.show_on_open": false })).toBe(false);
  });
});

describe("saveSettings", () => {
  it("PATCHes the settings route and returns effective values", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ values: { colorScheme: "dark" }, locked: [] }), { status: 200 })
    );
    const out = await saveSettings({ reader: { highlightColor: "blue" } });
    expect(out.values.colorScheme).toBe("dark");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/users/me/settings");
    expect(init?.method).toBe("PATCH");
    expect(JSON.parse(String(init?.body))).toEqual({ reader: { highlightColor: "blue" } });
  });
  it("surfaces the API error message", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "suggestions.show_on_open is locked for your study group" }), { status: 409 })
    );
    await expect(saveSettings({ "suggestions.show_on_open": false })).rejects.toThrow("locked");
  });
});

it("exposes the exact confirmation phrase the API expects", () => {
  expect(DELETE_ACCOUNT_CONFIRMATION).toBe("delete my account");
});
