// text-anchor.ts — locate a text quote inside a page's text-layer segments.
//
// Used for two things: (1) flashing a SELAR evidence passage or a compare
// quote after a jump, and (2) re-anchoring a saved highlight after zoom,
// re-render or a pdf.js upgrade. Matching ignores whitespace, case and
// typographic quote/dash variants, because the worker's extracted chunk text
// and pdf.js's text items rarely agree on spacing.

export interface SegmentPoint {
  segment: number;
  offset: number;
}

export interface QuoteMatch {
  start: SegmentPoint;
  /** Exclusive end. */
  end: SegmentPoint;
  /** Position in the compact (whitespace-free) page text; store as a hint. */
  index: number;
  /** Number of quote characters matched (shorter than the quote on fallback). */
  matched: number;
}

export interface QuoteContext {
  prefix?: string;
  suffix?: string;
  /** Compact index where the quote was last found. */
  hint?: number;
}

const QUOTES: Record<string, string> = {
  "\u2018": "'", "\u2019": "'", "\u201a": "'", "\u201b": "'",
  "\u201c": '"', "\u201d": '"', "\u201e": '"', "\u201f": '"',
  "\u2010": "-", "\u2011": "-", "\u2012": "-", "\u2013": "-", "\u2014": "-", "\u2015": "-", "\u2212": "-",
  "\u00a0": " ", "\ufb01": "fi", "\ufb02": "fl",
};

function foldChar(char: string): string {
  return (QUOTES[char] ?? char).toLowerCase();
}

/** Normalised text with single spaces plus a map back to original indices. */
export function normalizeForSearch(input: string): { text: string; map: number[] } {
  let text = "";
  const map: number[] = [];
  let pendingSpace = false;
  for (let i = 0; i < input.length; i++) {
    const char = input[i];
    // "retrie-\nval" → "retrieval"
    if (char === "-" && /\S/.test(input[i - 1] || "") && /^-[ \t]*\r?\n\s*\S/.test(input.slice(i, i + 8))) {
      while (i + 1 < input.length && /\s/.test(input[i + 1])) i++;
      continue;
    }
    if (/\s/.test(char)) {
      pendingSpace = text.length > 0;
      continue;
    }
    if (pendingSpace) {
      text += " ";
      map.push(i);
      pendingSpace = false;
    }
    for (const folded of foldChar(char)) {
      text += folded;
      map.push(i);
    }
  }
  return { text, map };
}

interface Compact {
  text: string;
  points: SegmentPoint[];
}

function compactSegments(segments: string[]): Compact {
  let text = "";
  const points: SegmentPoint[] = [];
  segments.forEach((segment, index) => {
    for (let offset = 0; offset < segment.length; offset++) {
      const char = segment[offset];
      if (/\s/.test(char)) continue;
      for (const folded of foldChar(char)) {
        if (/\s/.test(folded)) continue;
        text += folded;
        points.push({ segment: index, offset });
      }
    }
  });
  return { text, points };
}

export function compact(value: string): string {
  let text = "";
  for (const char of value) {
    if (/\s/.test(char)) continue;
    for (const folded of foldChar(char)) if (!/\s/.test(folded)) text += folded;
  }
  return text;
}

function occurrences(haystack: string, needle: string): number[] {
  const found: number[] = [];
  if (!needle) return found;
  let from = 0;
  while (from <= haystack.length - needle.length) {
    const at = haystack.indexOf(needle, from);
    if (at < 0) break;
    found.push(at);
    from = at + 1;
  }
  return found;
}

function commonSuffix(a: string, b: string): number {
  let n = 0;
  while (n < a.length && n < b.length && a[a.length - 1 - n] === b[b.length - 1 - n]) n++;
  return n;
}

function commonPrefix(a: string, b: string): number {
  let n = 0;
  while (n < a.length && n < b.length && a[n] === b[n]) n++;
  return n;
}

const MIN_FALLBACK = 20;
const CONTEXT = 32;

export function locateQuote(segments: string[], quote: string, context: QuoteContext = {}): QuoteMatch | null {
  const page = compactSegments(segments);
  const needle = compact(quote);
  if (!needle || !page.text) return null;

  let length = needle.length;
  let candidates = occurrences(page.text, needle);
  if (!candidates.length && needle.length > MIN_FALLBACK) {
    // Binary search the longest leading part of the quote present on the page.
    let lo = MIN_FALLBACK;
    let hi = needle.length - 1;
    let best = 0;
    while (lo <= hi) {
      const mid = (lo + hi) >> 1;
      if (page.text.includes(needle.slice(0, mid))) {
        best = mid;
        lo = mid + 1;
      } else {
        hi = mid - 1;
      }
    }
    if (best) {
      length = best;
      candidates = occurrences(page.text, needle.slice(0, best));
    }
  }
  if (!candidates.length) return null;

  const prefix = compact(context.prefix || "").slice(-CONTEXT);
  const suffix = compact(context.suffix || "").slice(0, CONTEXT);
  let bestIndex = candidates[0];
  let bestScore = -Infinity;
  for (const at of candidates) {
    let score = 0;
    if (prefix) score += commonSuffix(page.text.slice(Math.max(0, at - prefix.length), at), prefix);
    if (suffix) score += commonPrefix(page.text.slice(at + length, at + length + suffix.length), suffix);
    if (context.hint !== undefined) score -= Math.abs(at - context.hint) / 1e6;
    if (score > bestScore) {
      bestScore = score;
      bestIndex = at;
    }
  }

  const first = page.points[bestIndex];
  const last = page.points[bestIndex + length - 1];
  return {
    start: { ...first },
    end: { segment: last.segment, offset: last.offset + 1 },
    index: bestIndex,
    matched: length,
  };
}

export function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/** HTML for a text-layer item with case-insensitive matches of `query` wrapped in <mark>. */
export function markMatches(text: string, query: string): string {
  const needle = query.trim().toLowerCase();
  if (!needle) return escapeHtml(text);
  const lower = text.toLowerCase();
  if (lower.length !== text.length) return escapeHtml(text);
  let html = "";
  let cursor = 0;
  for (let at = lower.indexOf(needle); at >= 0; at = lower.indexOf(needle, at + needle.length)) {
    html += escapeHtml(text.slice(cursor, at)) + `<mark class="rd-find">${escapeHtml(text.slice(at, at + needle.length))}</mark>`;
    cursor = at + needle.length;
  }
  return html + escapeHtml(text.slice(cursor));
}
