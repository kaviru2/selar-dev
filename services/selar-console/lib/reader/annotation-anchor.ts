import { compact, locateQuote, type QuoteMatch } from "./text-anchor";

/** Offsets are UTF-16 positions in the concatenated page text-layer spans. */
export interface TextAnchor {
  version: 1;
  source_hash: string;
  exact: string;
  prefix: string;
  suffix: string;
  start: number;
  end: number;
}

export function makeTextAnchor(text: string, start: number, end: number, source: string): TextAnchor | null {
  if (!source || !Number.isInteger(start) || !Number.isInteger(end) || start < 0 || end <= start || end > text.length || !text.slice(start,end).trim()) return null;
  return { version: 1, source_hash: source, exact: text.slice(start,end), prefix: text.slice(Math.max(0,start-64),start), suffix: text.slice(end,end+64), start, end };
}

export function resolveTextAnchor(segments: string[], anchor: TextAnchor, source: string): QuoteMatch | null {
  if (!source || anchor.source_hash !== source || anchor.version !== 1) return null;
  const match = locateQuote(segments, anchor.exact, { prefix: anchor.prefix, suffix: anchor.suffix, hint: compact(segments.join("").slice(0,anchor.start)).length });
  // Evidence jumps allow a partial quote; user marks must never silently shorten it.
  return match?.matched === compact(anchor.exact).length ? match : null;
}
