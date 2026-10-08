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
