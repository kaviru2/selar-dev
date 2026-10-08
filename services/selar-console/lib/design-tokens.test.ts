// design-tokens.test.ts — guards the brand palette's WCAG AA contrast and
// keeps tokens.css the single source for colours.

import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const css = readFileSync(path.resolve(__dirname, "../app/tokens.css"), "utf8");

function token(name: string): string {
  const match = css.match(new RegExp(`--${name}:\\s*(#[0-9a-fA-F]{6})`));
  if (!match) throw new Error(`token --${name} not found`);
  return match[1];
}

function luminance(hex: string) {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrast(a: string, b: string) {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

describe("SELAR design tokens", () => {
  it("keeps the deck palette values", () => {
    expect(token("selar-green-800").toLowerCase()).toBe("#1f5135");
    expect(token("selar-green-500").toLowerCase()).toBe("#5b8a6f");
    expect(token("selar-ink").toLowerCase()).toBe("#0f172a");
    expect(token("selar-slate").toLowerCase()).toBe("#475569");
    expect(token("selar-light").toLowerCase()).toBe("#f1f5f9");
    expect(token("selar-sky").toLowerCase()).toBe("#e1edfb");
    expect(token("selar-rust").toLowerCase()).toBe("#b4532a");
    expect(token("selar-rust-tint").toLowerCase()).toBe("#fbede6");
    expect(token("selar-amber").toLowerCase()).toBe("#d97706");
  });

  it.each([
    ["selar-ink", "selar-light"],
    ["selar-ink", "selar-white"],
    ["selar-slate", "selar-white"],
    ["selar-slate", "selar-light"],
    ["selar-slate-500", "selar-white"],
    ["selar-green-800", "selar-white"],
    ["selar-green-800", "selar-light"],
    ["selar-green-600", "selar-white"],
    ["selar-rust-700", "selar-rust-tint"],
    ["selar-rust-700", "selar-white"],
    ["selar-amber-800", "selar-amber-tint"],
    ["selar-sky-600", "selar-sky"],
    ["selar-ink", "selar-sky"],
    ["selar-danger", "selar-white"],
  ])("text --%s on --%s meets AA (4.5:1)", (fg, bg) => {
    expect(contrast(token(fg), token(bg))).toBeGreaterThanOrEqual(4.5);
  });

  it("white text on the primary button meets AA", () => {
    expect(contrast("#ffffff", token("selar-green-800"))).toBeGreaterThanOrEqual(4.5);
    expect(contrast("#ffffff", token("selar-rust-700"))).toBeGreaterThanOrEqual(4.5);
  });

  it("honours reduced motion globally", () => {
    expect(css).toMatch(/@media \(prefers-reduced-motion: reduce\)/);
  });
});

/** Reads a hex token from a given block (":root {" light or '[data-theme="dark"] {'). */
function blockTokens(selector: string): Record<string, string> {
  const out: Record<string, string> = {};
  let idx = css.indexOf(selector);
  while (idx !== -1) {
    const body = css.slice(idx, css.indexOf("}", idx));
    for (const m of body.matchAll(/--([a-z0-9-]+):\s*(#[0-9a-fA-F]{6})\b/g)) out[m[1]] = m[2];
    idx = css.indexOf(selector, idx + 1);
  }
  return out;
}

describe("semantic tokens meet WCAG AA in both themes", () => {
  const brand = blockTokens(":root {");
  const darkT = blockTokens('[data-theme="dark"] {');
  // Light semantic tokens reference brand tokens; resolve the ones we test.
  const light: Record<string, string> = {
    bg: brand["selar-white"], "bg-2": brand["selar-paper"], "bg-3": brand["selar-light"], "bg-raised": brand["selar-white"],
    ink: brand["selar-ink"], "ink-3": brand["selar-slate"], "ink-4": brand["selar-slate-500"],
    accent: brand["selar-green-800"], "accent-2": brand["selar-green-600"], "on-accent": "#ffffff",
    "panel-brand": brand["selar-green-800"], "panel-brand-ink": "#ffffff", "panel-brand-muted": "#dbe7df",
    "accent-warm-ink": brand["selar-rust-700"], "accent-warm-tint": brand["selar-rust-tint"],
    "tint-green": brand["selar-green-50"], "tint-green-2": brand["selar-green-100"],
    "tint-sky": brand["selar-sky"], "text-sky": brand["selar-sky-600"],
    "tint-amber": brand["selar-amber-tint"], "text-amber": brand["selar-amber-800"], "text-amber-strong": "#78350f",
    error: brand["selar-danger"],
  };
  const pairs: Array<[string, string]> = [
    ["ink", "bg"], ["ink", "bg-2"], ["ink", "bg-3"], ["ink", "bg-raised"],
    ["ink-3", "bg"], ["ink-3", "bg-2"], ["ink-3", "bg-3"], ["ink-3", "bg-raised"],
    ["ink-4", "bg"],
    ["accent", "bg"], ["accent", "bg-3"], ["accent", "tint-green-2"], ["accent-2", "bg"], ["accent-2", "bg-3"],
    ["on-accent", "accent"],
    ["panel-brand-ink", "panel-brand"], ["panel-brand-muted", "panel-brand"],
    ["accent-warm-ink", "accent-warm-tint"],
    ["text-sky", "tint-sky"], ["text-amber", "tint-amber"], ["text-amber-strong", "tint-amber"],
    ["ink", "tint-sky"], ["ink-3", "tint-sky"], ["ink-3", "tint-green"],
    ["error", "bg"],
  ];
  it.each(pairs)("light: --%s on --%s", (fg, bg) => {
    expect(contrast(light[fg], light[bg])).toBeGreaterThanOrEqual(4.5);
  });
  it.each(pairs)("dark: --%s on --%s", (fg, bg) => {
    const f = darkT[fg] ?? light[fg];
    const b = darkT[bg] ?? light[bg];
    expect(contrast(f, b)).toBeGreaterThanOrEqual(4.5);
  });
  it("defines every tested token in the dark block", () => {
    const missing = Array.from(new Set(pairs.flat())).filter((t) => !darkT[t] && !["panel-brand-ink", "panel-brand-muted"].includes(t));
    expect(missing).toEqual([]);
  });
});

describe("dark theme plumbing", () => {
  it("declares color-scheme for native controls", () => {
    expect(css).toMatch(/\[data-theme="dark"\] \{\s*color-scheme: dark;/);
    expect(css).toMatch(/:root \{\s*color-scheme: light;/);
  });
});
