import { describe, expect, it } from "vitest";
import {
  clampPanelWidth,
  DEFAULT_LAYOUT,
  initialLayout,
  isTypingTarget,
  keyboardResize,
  MIN_DOCUMENT_WIDTH,
  OVERLAY_BREAKPOINT,
  PANEL_LIMITS,
  parseStoredLayout,
} from "./reader-layout";

describe("clampPanelWidth", () => {
  it("keeps widths inside each panel's limits", () => {
    for (const side of ["library", "connections"] as const) {
      const { min, max } = PANEL_LIMITS[side];
      expect(clampPanelWidth(side, min - 100)).toBe(min);
      expect(clampPanelWidth(side, max + 100)).toBe(max);
      expect(clampPanelWidth(side, (min + max) / 2)).toBe(Math.round((min + max) / 2));
    }
  });

  it("leaves the document its minimum width when the viewport is tight", () => {
    const viewport = 1170;
    const library = 240;
    const width = clampPanelWidth("connections", 10_000, viewport, library);
    expect(viewport - library - width).toBeGreaterThanOrEqual(MIN_DOCUMENT_WIDTH);
  });

  it("never goes below the panel minimum even if the viewport cannot fit it", () => {
    expect(clampPanelWidth("connections", 500, 600, 240)).toBe(PANEL_LIMITS.connections.min);
  });

  it("falls back to the initial width for non-finite input", () => {
    expect(clampPanelWidth("library", Number.NaN)).toBe(PANEL_LIMITS.library.initial);
  });
});

describe("initialLayout", () => {
  it("opens both panels on wide windows", () => {
    expect(initialLayout(1600)).toEqual(DEFAULT_LAYOUT);
  });
  it("starts with Connections closed on medium windows like the reported 1170px", () => {
    const layout = initialLayout(1170);
    expect(layout.libraryOpen).toBe(true);
    expect(layout.connectionsOpen).toBe(false);
  });
  it("starts with both panels closed below the overlay breakpoint", () => {
    const layout = initialLayout(OVERLAY_BREAKPOINT - 1);
    expect(layout.libraryOpen || layout.connectionsOpen).toBe(false);
  });
});

describe("parseStoredLayout", () => {
  const fallback = initialLayout(1170);
  it("restores a saved layout and re-clamps widths", () => {
    const saved = JSON.stringify({ libraryOpen: false, connectionsOpen: true, libraryWidth: 9999, connectionsWidth: 300 });
    expect(parseStoredLayout(saved, fallback)).toEqual({
      libraryOpen: false,
      connectionsOpen: true,
      libraryWidth: PANEL_LIMITS.library.max,
      connectionsWidth: 300,
    });
  });
  it("uses the fallback for missing, malformed or partial data", () => {
    expect(parseStoredLayout(null, fallback)).toEqual(fallback);
    expect(parseStoredLayout("{not json", fallback)).toEqual(fallback);
    expect(parseStoredLayout(JSON.stringify({ libraryOpen: "yes" }), fallback)).toEqual(fallback);
  });
});

describe("keyboardResize", () => {
  it("grows each panel toward the document", () => {
    expect(keyboardResize("library", 240, "ArrowRight")).toBeGreaterThan(240);
    expect(keyboardResize("connections", 360, "ArrowLeft")).toBeGreaterThan(360);
    expect(keyboardResize("library", 240, "ArrowLeft")).toBeLessThan(240);
    expect(keyboardResize("connections", 360, "ArrowRight")).toBeLessThan(360);
  });
  it("takes bigger steps with Shift and jumps with Home/End", () => {
    const small = keyboardResize("library", 240, "ArrowRight")! - 240;
    const big = keyboardResize("library", 240, "ArrowRight", true)! - 240;
    expect(big).toBeGreaterThan(small);
    expect(keyboardResize("connections", 360, "Home")).toBe(PANEL_LIMITS.connections.min);
    expect(keyboardResize("connections", 360, "End")).toBe(PANEL_LIMITS.connections.max);
  });
  it("ignores other keys", () => {
    expect(keyboardResize("library", 240, "a")).toBeNull();
  });
});

describe("isTypingTarget", () => {
  it("is true for text fields and false for buttons", () => {
    expect(isTypingTarget(document.createElement("textarea"))).toBe(true);
    expect(isTypingTarget(document.createElement("input"))).toBe(true);
    expect(isTypingTarget(document.createElement("button"))).toBe(false);
    expect(isTypingTarget(null)).toBe(false);
  });
});
