import { describe, expect, it } from "vitest";
import {
  buildLayout,
  captureAnchor,
  clampPage,
  fitScale,
  keyAction,
  pageAtOffset,
  parsePageTarget,
  renderRange,
  scrollTopForAnchor,
  scrollTopForPage,
} from "./navigation";

const sizes = Array.from({ length: 10 }, () => ({ width: 600, height: 800 }));

describe("clampPage", () => {
  it("keeps the page inside 1..numPages and rounds non-integers", () => {
    expect(clampPage(0, 10)).toBe(1);
    expect(clampPage(11, 10)).toBe(10);
    expect(clampPage(4.6, 10)).toBe(5);
    expect(clampPage(Number.NaN, 10)).toBe(1);
  });
  it("accepts any positive page while the page count is still unknown", () => {
    expect(clampPage(7, 0)).toBe(7);
  });
});

describe("parsePageTarget", () => {
  it("prefers ?page= and falls back to #page=", () => {
    expect(parsePageTarget("?docId=a&page=4", "")).toBe(4);
    expect(parsePageTarget("?docId=a", "#page=9")).toBe(9);
    expect(parsePageTarget("?docId=a&page=2", "#page=9")).toBe(2);
  });
  it("ignores missing, zero, negative and non-numeric values", () => {
    expect(parsePageTarget("?docId=a", "")).toBeNull();
    expect(parsePageTarget("?page=0", "")).toBeNull();
    expect(parsePageTarget("?page=-3", "")).toBeNull();
    expect(parsePageTarget("?page=abc", "#page=x")).toBeNull();
  });
});

describe("buildLayout", () => {
  it("stacks pages vertically with a gap and padding at the given scale", () => {
    const layout = buildLayout(sizes.slice(0, 3), 0.5, { gap: 10, padding: 20 });
    expect(layout.tops).toEqual([20, 20 + 400 + 10, 20 + 2 * (400 + 10)]);
    expect(layout.heights).toEqual([400, 400, 400]);
    expect(layout.widths).toEqual([300, 300, 300]);
    expect(layout.totalHeight).toBe(20 + 3 * 400 + 2 * 10 + 20);
  });
});

describe("pageAtOffset", () => {
  const layout = buildLayout(sizes, 1, { gap: 16, padding: 24 });
  it("reports the page under the reading line (a third of the way down the viewport)", () => {
    expect(pageAtOffset(layout, 0, 900)).toBe(1);
    // reading line = scrollTop + 300; page 2 starts at 24 + 816 = 840.
    expect(pageAtOffset(layout, 520, 900)).toBe(1);
    expect(pageAtOffset(layout, 541, 900)).toBe(2);
  });
  it("treats the gap between pages as belonging to the next page", () => {
    // reading line at 830 (inside the 16px gap before page 2)
    expect(pageAtOffset(layout, 530, 900)).toBe(2);
  });
  it("reports the last page when scrolled to the very end", () => {
    expect(pageAtOffset(layout, layout.totalHeight - 900, 900)).toBe(10);
  });
});

describe("renderRange", () => {
  const layout = buildLayout(sizes, 1, { gap: 16, padding: 24 });
  it("renders only pages intersecting the viewport plus an overscan", () => {
    expect(renderRange(layout, 0, 900, 1)).toEqual([1, 3]);
    const top5 = layout.tops[4];
    expect(renderRange(layout, top5 + 10, 900, 1)).toEqual([4, 7]);
  });
  it("never goes outside the document", () => {
    expect(renderRange(layout, layout.totalHeight, 900, 2)).toEqual([8, 10]);
  });
});

describe("fitScale", () => {
  it("fit-width fills the available width minus padding", () => {
    expect(fitScale("fit-width", { width: 600, height: 800 }, { width: 648, height: 900 }, 24)).toBeCloseTo(1);
  });
  it("fit-page fits the whole page in the viewport", () => {
    expect(fitScale("fit-page", { width: 600, height: 800 }, { width: 2000, height: 448 }, 24)).toBeCloseTo(0.5);
  });
  it("clamps to a sane range for tiny or huge containers", () => {
    expect(fitScale("fit-width", { width: 600, height: 800 }, { width: 10, height: 10 }, 24)).toBe(0.25);
    expect(fitScale("fit-width", { width: 100, height: 100 }, { width: 5000, height: 900 }, 24)).toBe(4);
  });
});

describe("zoom anchoring", () => {
  it("keeps the same point of the same page at the top after a zoom change", () => {
    const before = buildLayout(sizes, 1, { gap: 16, padding: 24 });
    const scrollTop = before.tops[3] + 200; // 25% into page 4
    const anchor = captureAnchor(before, scrollTop);
    expect(anchor.page).toBe(4);
    expect(anchor.fraction).toBeCloseTo(0.25);
    const after = buildLayout(sizes, 2, { gap: 16, padding: 24 });
    expect(scrollTopForAnchor(after, anchor)).toBeCloseTo(after.tops[3] + 400);
  });
  it("scrollTopForPage puts the top of the page just below the top edge", () => {
    const layout = buildLayout(sizes, 1, { gap: 16, padding: 24 });
    expect(scrollTopForPage(layout, 3)).toBe(layout.tops[2] - 8);
    expect(scrollTopForPage(layout, 1)).toBe(0);
  });
});

describe("keyAction", () => {
  const key = (k: string, extra: Partial<{ ctrlKey: boolean; metaKey: boolean; altKey: boolean; shiftKey: boolean; inEditable: boolean }> = {}) =>
    keyAction({ key: k, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, inEditable: false, ...extra });
  it("maps paging keys", () => {
    expect(key("ArrowRight")).toBe("next");
    expect(key("ArrowLeft")).toBe("prev");
    expect(key("PageDown")).toBe("next");
    expect(key("PageUp")).toBe("prev");
    expect(key("Home")).toBe("first");
    expect(key("End")).toBe("last");
    // [ and ] toggle the side panels (lib/reader-layout.ts), not pages.
    expect(key("]")).toBeNull();
    expect(key("[")).toBeNull();
  });
  it("maps find and zoom shortcuts", () => {
    expect(key("f", { ctrlKey: true })).toBe("find");
    expect(key("f", { metaKey: true })).toBe("find");
    expect(key("=", { ctrlKey: true })).toBe("zoomIn");
    expect(key("+", { metaKey: true })).toBe("zoomIn");
    expect(key("-", { ctrlKey: true })).toBe("zoomOut");
    expect(key("0", { ctrlKey: true })).toBe("zoomReset");
  });
  it("leaves typing alone inside inputs, textareas and editors", () => {
    expect(key("ArrowRight", { inEditable: true })).toBeNull();
    expect(key("Home", { inEditable: true })).toBeNull();
    expect(key("f", { ctrlKey: true, inEditable: true })).toBe("find");
  });
  it("ignores unmodified letters and modified paging keys", () => {
    expect(key("f")).toBeNull();
    expect(key("j")).toBeNull();
    expect(key("ArrowRight", { altKey: true })).toBeNull();
    expect(key("ArrowLeft", { metaKey: true })).toBeNull();
  });
});
