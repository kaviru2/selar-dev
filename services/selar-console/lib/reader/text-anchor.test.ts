import { describe, expect, it } from "vitest";
import { escapeHtml, locateQuote, markMatches, normalizeForSearch } from "./text-anchor";

describe("normalizeForSearch", () => {
  it("lower-cases, folds whitespace and typographic quotes/dashes, and keeps an index map", () => {
    const { text, map } = normalizeForSearch("  The  “Testing”\nEffect—now ");
    expect(text).toBe("the \"testing\" effect-now");
    // every normalised char maps back to a position in the original string
    expect(map).toHaveLength(text.length);
    expect("  The  “Testing”\nEffect—now "[map[0]]).toBe("T");
    expect("  The  “Testing”\nEffect—now "[map[text.indexOf("-now") - 1]]).toBe("t");
  });

  it("joins words hyphenated across a line break", () => {
    expect(normalizeForSearch("retrie-\nval practice").text).toBe("retrieval practice");
  });
});

describe("locateQuote", () => {
  const segments = [
    "Section 2: Interleaved Practice",
    "Paragraph 1 on page 2 describes interleaved practice in an invented garden study.",
    "The fabricated learners practised interleaved practice with toy flashcards ",
    "and paper boats.",
  ];

  it("finds a quote spanning several text-layer segments and returns segment offsets", () => {
    const hit = locateQuote(segments, "with toy flashcards and paper boats");
    expect(hit).not.toBeNull();
    expect(hit!.start).toEqual({ segment: 2, offset: segments[2].indexOf("with") });
    expect(hit!.end).toEqual({ segment: 3, offset: "and paper boats".length });
  });

  it("is robust to whitespace, case and curly quotes differences", () => {
    const hit = locateQuote(segments, "THE   fabricated learners");
    expect(hit!.start).toEqual({ segment: 2, offset: 0 });
  });

  it("uses prefix/suffix context to choose between repeated matches", () => {
    const hit = locateQuote(segments, "interleaved practice", { prefix: "learners practised " });
    expect(hit!.start.segment).toBe(2);
    const first = locateQuote(segments, "interleaved practice", { prefix: "describes " });
    expect(first!.start.segment).toBe(1);
  });

  it("prefers the match nearest the remembered offset when no context distinguishes them", () => {
    const hit = locateQuote(segments, "interleaved practice", { hint: 9999 });
    expect(hit!.start.segment).toBe(2);
  });

  it("falls back to the longest leading part of a long quote that was cut by the extractor", () => {
    const hit = locateQuote(segments, "The fabricated learners practised interleaved practice with toy flashcards and paper boats. Gradient descent text that is not on this page at all");
    expect(hit!.start).toEqual({ segment: 2, offset: 0 });
  });

  it("returns null when nothing usable matches", () => {
    expect(locateQuote(segments, "completely unrelated sentence about volcanoes")).toBeNull();
    expect(locateQuote(segments, "   ")).toBeNull();
  });
});

describe("markMatches / escapeHtml", () => {
  it("escapes HTML in text-layer strings", () => {
    expect(escapeHtml(`<b>"x" & 'y'</b>`)).toBe("&lt;b&gt;&quot;x&quot; &amp; &#39;y&#39;&lt;/b&gt;");
  });
  it("wraps case-insensitive matches in <mark> without injecting markup from the text", () => {
    expect(markMatches("Spaced <retrieval> and SPACED practice", "spaced"))
      .toBe('<mark class="rd-find">Spaced</mark> &lt;retrieval&gt; and <mark class="rd-find">SPACED</mark> practice');
    expect(markMatches("nothing here", "")).toBe("nothing here");
  });
});
