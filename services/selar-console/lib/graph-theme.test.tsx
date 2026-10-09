import { act } from "react";
import { createRoot } from "react-dom/client";
import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, expect, it, vi } from "vitest";
import { NODE_TYPE_COLORS, REL_COLORS, useGraphColors } from "./graph-theme";

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); document.documentElement.removeAttribute("data-theme"); });

it("resolves semantic canvas colours again on theme changes without changing the graph data", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.spyOn(window, "getComputedStyle").mockImplementation(() => ({ getPropertyValue: (name: string) =>
    name === "--text-sky" ? (document.documentElement.dataset.theme === "dark" ? "#bcd5f2" : "#2f5f8f") : "#475569",
  }) as CSSStyleDeclaration);
  let resolve: (token: string) => string = () => "";
  function Probe() { resolve = useGraphColors(); return null; }
  const root = createRoot(document.createElement("div"));
  try {
    await act(async () => root.render(<Probe />));
    expect(resolve(NODE_TYPE_COLORS.concept)).toBe("#2f5f8f");
    expect(resolve(REL_COLORS.concept_overlap)).toBe("#2f5f8f");
    await act(async () => { document.documentElement.dataset.theme = "dark"; });
    expect(resolve(NODE_TYPE_COLORS.concept)).toBe("#bcd5f2");
    expect(resolve(REL_COLORS.concept_overlap)).toBe("#bcd5f2");
    for (const value of [...Object.values(NODE_TYPE_COLORS), ...Object.values(REL_COLORS)]) expect(value).toMatch(/^var\(--[a-z0-9-]+\)$/);
  } finally { await act(async () => root.unmount()); }
});

it("keeps graph surfaces and drawing colours on semantic roles, not a light-only palette", () => {
  const page = readFileSync(path.resolve(__dirname, "../app/(app)/graph/page.tsx"), "utf8");
  expect(page).not.toMatch(/["'`]#[0-9a-f]{3,8}\b|rgba?\(/i);
  expect(page).not.toMatch(/\$\{(?:node\.color|selectedNode\.color|relColor)\}[0-9a-f]{2}/);
  const css = readFileSync(path.resolve(__dirname, "../app/globals.css"), "utf8");
  const layers = css.slice(css.indexOf(".graph-layer-toggle {"), css.indexOf("/* ——— Quiz View"));
  expect(layers).not.toMatch(/#[0-9a-f]{6}\b|rgba?\(/i);
});
