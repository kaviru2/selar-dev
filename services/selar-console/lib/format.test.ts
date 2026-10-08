import { describe, expect, it } from "vitest";
import { pageCountLabel, nextMatchLabel } from "./format";

describe("pageCountLabel", () => {
  it("uses the singular for one page", () => {
    expect(pageCountLabel(1)).toBe("1 page");
  });
  it("uses the plural otherwise", () => {
    expect(pageCountLabel(2)).toBe("2 pages");
    expect(pageCountLabel(0)).toBe("0 pages");
  });
});

describe("nextMatchLabel", () => {
  it("says there are no suggestions instead of offering a jump", () => {
    expect(nextMatchLabel(0, 0)).toBe("No suggestions");
  });
  it("offers the next match when suggestions exist elsewhere", () => {
    expect(nextMatchLabel(3, 0)).toBe("Next match ›");
  });
  it("counts highlights on the current page", () => {
    expect(nextMatchLabel(3, 2)).toBe("2 highlighted · Next ›");
  });
});
