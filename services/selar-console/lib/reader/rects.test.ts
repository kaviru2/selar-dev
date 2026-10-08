import { describe, expect, it } from "vitest";
import { mergeLineRects, normalizeRects } from "./rects";

const r = (left: number, top: number, width: number, height: number) => ({ left, top, width, height, right: left + width, bottom: top + height });

describe("mergeLineRects", () => {
  it("merges overlapping or touching rects on the same line into one box", () => {
    const merged = mergeLineRects([r(10, 100, 50, 12), r(58, 101, 40, 11), r(100, 100, 30, 12)]);
    expect(merged).toEqual([{ left: 10, top: 100, width: 120, height: 12 }]);
  });

  it("keeps separate lines separate and orders them top to bottom", () => {
    const merged = mergeLineRects([r(10, 130, 50, 12), r(10, 100, 80, 12)]);
    expect(merged).toEqual([
      { left: 10, top: 100, width: 80, height: 12 },
      { left: 10, top: 130, width: 50, height: 12 },
    ]);
  });

  it("drops empty and hairline rects produced by line breaks", () => {
    expect(mergeLineRects([r(10, 100, 0, 12), r(10, 100, 40, 0.5), r(10, 100, 40, 12)])).toEqual([
      { left: 10, top: 100, width: 40, height: 12 },
    ]);
  });

  it("drops rects that fully contain other lines (pdf.js endOfContent artefacts)", () => {
    const merged = mergeLineRects([r(10, 100, 80, 12), r(10, 116, 80, 12), r(0, 90, 600, 400)]);
    expect(merged).toHaveLength(2);
  });
});

describe("normalizeRects", () => {
  it("converts client rects to 0..1 page coordinates, clamped to the page", () => {
    const page = { left: 100, top: 50, width: 600, height: 800 };
    expect(normalizeRects([{ left: 160, top: 130, width: 300, height: 16 }], page)).toEqual([
      { x: 0.1, y: 0.1, w: 0.5, h: 0.02 },
    ]);
    const [clamped] = normalizeRects([{ left: 50, top: 40, width: 800, height: 20 }], page);
    expect(clamped.x).toBe(0);
    expect(clamped.y).toBe(0);
    expect(clamped.x + clamped.w).toBeLessThanOrEqual(1);
  });
});
