import { describe, expect, it } from "vitest";
import { visibleSuggestionBoxes } from "./reader-highlights";

describe("visibleSuggestionBoxes", () => {
  it("returns unique, valid boxes for the current page only", () => {
    const boxes = visibleSuggestionBoxes([
      { id: "one", status: "pending", src_page: 2, src_bboxes: [{ x: 0.1, y: 0.2, w: 0.2, h: 0.1 }] },
      { id: "two", status: "pending", src_page: 2, src_bboxes: [{ x: 0.1, y: 0.2, w: 0.2, h: 0.1 }] },
      { id: "three", status: "pending", src_page: 3, src_bboxes: [{ x: 0.4, y: 0.2, w: 0.1, h: 0.1 }] },
      { id: "four", status: "rejected", src_page: 2, src_bboxes: [{ x: 0.3, y: 0.2, w: 0.1, h: 0.1 }] },
    ], 2);

    expect(boxes).toEqual([
      { suggestionID: "one", bbox: { x: 0.1, y: 0.2, w: 0.2, h: 0.1 } },
    ]);
  });

  it("drops malformed, out-of-bounds, and oversized boxes", () => {
    const boxes = visibleSuggestionBoxes([
      { id: "bad", status: "pending", src_page: 1, src_bboxes: [
        { x: -0.1, y: 0.2, w: 0.1, h: 0.1 },
        { x: 0.9, y: 0.2, w: 0.2, h: 0.1 },
        { x: 0.1, y: 0.1, w: 0.8, h: 0.8 },
        { x: 0.1, y: 0.1, w: 0.1, h: 0.1 },
      ] },
    ], 1);

    expect(boxes).toEqual([
      { suggestionID: "bad", bbox: { x: 0.1, y: 0.1, w: 0.1, h: 0.1 } },
    ]);
  });
});
