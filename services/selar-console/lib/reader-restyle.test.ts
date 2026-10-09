// Issue #77 item 5: Reader, reflection prompts, reading practice and the
// graph/progress pages stay on the shared design tokens and UI kit.
import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const root = path.resolve(__dirname, "..");
const read = (file: string) => readFileSync(path.join(root, file), "utf8");
const HARD_COLOUR = /#[0-9a-f]{3,8}\b|rgba?\(/i;

const COMPONENTS = [
  "components/ConnectionsPanel.tsx",
  "components/ReadingPractice.tsx",
  "app/(app)/progress/page.tsx",
  "app/(app)/review/page.tsx",
];

function allCss(): string {
  const files = readdirSync(path.join(root, "app")).filter((f) => f.endsWith(".css")).map((f) => `app/${f}`);
  files.push("components/reader/reader.css", "components/reader/annotation-tools.css");
  return files.map(read).join("\n");
}

function staticClasses(source: string): string[] {
  const names = new Set<string>();
  for (const m of source.matchAll(/className="([^"]+)"/g)) for (const c of m[1].split(/\s+/)) names.add(c);
  return [...names];
}

describe("reader restyle (#77)", () => {
  it.each(COMPONENTS)("%s uses classes, not inline styles or hard-coded colours", (file) => {
    const src = read(file);
    expect(src).not.toMatch(/style=\{\{/);
    expect(src).not.toMatch(HARD_COLOUR);
  });

  it.each(COMPONENTS)("every static class in %s is defined in a stylesheet", (file) => {
    const css = allCss();
    const missing = staticClasses(read(file)).filter((c) => !new RegExp(`\\.${c.replace(/[-]/g, "\\-")}(?![\\w-])`).test(css));
    expect(missing).toEqual([]);
  });

  it("styles the reflection prompt with the guided-connections kit", () => {
    const src = read("components/ConnectionsPanel.tsx");
    for (const cls of ["cx-title", "cx-reflection", "cx-ta", "cx-btn cx-sec", "cx-quote", "cx-error"]) expect(src).toContain(cls);
  });

  it("uses the shared button and card kit for reading practice", () => {
    const src = read("components/ReadingPractice.tsx");
    expect(src).toContain("buttonClass(");
    expect(src).toContain("ui-card");
  });

  it("keeps reader and connections CSS on tokens", () => {
    const css = read("app/globals.css");
    const reader = css.slice(css.indexOf("/* ——— Reader ——— */"), css.indexOf("/* ——— Library View ——— */"));
    const panels = css.slice(css.indexOf("/* ——— Reader side panels"), css.indexOf("/* Redesigned Graph page"));
    const cx = css.slice(css.indexOf("/* ——— Guided Connections sidebar"), css.indexOf("/* Brand mark"));
    for (const block of [reader, panels, cx]) {
      expect(block.length).toBeGreaterThan(100);
      expect(block).not.toMatch(HARD_COLOUR);
    }
    expect(read("app/practice.css")).not.toMatch(HARD_COLOUR);
    expect(read("app/(app)/reader/page.tsx")).not.toMatch(/whiteSpace/);
  });

  it("drops the legacy light-only reader overrides", () => {
    const css = read("app/globals.css");
    expect(css).not.toMatch(/\[data-theme="dark"\] \.pdf-page\b/);
    expect(read("components/reader/reader.css")).not.toMatch(/color: #fff/);
  });

  it("uses the shared empty state on the graph instead of an emoji", () => {
    const src = read("app/(app)/graph/page.tsx");
    expect(src).not.toContain("🕸");
    expect(src).toContain("<EmptyState");
    expect(src).not.toMatch(/neural network/i);
  });
});
