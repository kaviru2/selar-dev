// navigation.ts — pure navigation model for the continuous-scroll PDF reader.
//
// One model only: every page is laid out in a single vertical column at the
// current scale. Page changes, zoom, deep links and "jump to passage" are all
// expressed as a scrollTop on that column. Nothing here touches the DOM, so
// the behaviour is unit-tested and the component stays a thin shell.

export interface PageSize {
  width: number;
  height: number;
}

export interface Layout {
  tops: number[];
  heights: number[];
  widths: number[];
  totalHeight: number;
  maxWidth: number;
}

export type ZoomMode = "fit-width" | "fit-page" | number;

export const MIN_SCALE = 0.25;
export const MAX_SCALE = 4;
export const ZOOM_STEPS = [0.5, 0.67, 0.75, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3];
/** Fraction of the viewport height used as the "reading line". */
const READING_LINE = 1 / 3;
/** Pixels left above a page when jumping to it. */
const JUMP_MARGIN = 8;

export function clampPage(page: number, numPages: number): number {
  if (!Number.isFinite(page)) return 1;
  const rounded = Math.round(page);
  if (numPages <= 0) return Math.max(1, rounded);
  return Math.min(numPages, Math.max(1, rounded));
}

function positiveInt(value: string | null | undefined): number | null {
  if (!value || !/^\d+$/.test(value)) return null;
  const parsed = Number(value);
  return parsed >= 1 ? parsed : null;
}

/** Deep-link target from `?page=N` or, failing that, `#page=N` (pdf.js style). */
export function parsePageTarget(search: string, hash: string): number | null {
  const fromQuery = positiveInt(new URLSearchParams(search).get("page"));
  if (fromQuery) return fromQuery;
  const fromHash = positiveInt(new URLSearchParams(hash.replace(/^#/, "")).get("page"));
  return fromHash;
}

export function buildLayout(sizes: PageSize[], scale: number, { gap, padding }: { gap: number; padding: number }): Layout {
  const tops: number[] = [];
  const heights: number[] = [];
  const widths: number[] = [];
  let cursor = padding;
  let maxWidth = 0;
  sizes.forEach((size, index) => {
    const height = size.height * scale;
    const width = size.width * scale;
    tops.push(cursor);
    heights.push(height);
    widths.push(width);
    maxWidth = Math.max(maxWidth, width);
    cursor += height + (index < sizes.length - 1 ? gap : 0);
  });
  return { tops, heights, widths, totalHeight: cursor + padding, maxWidth };
}

/** Index (0-based) of the last page whose top is <= y. */
function indexAt(layout: Layout, y: number): number {
  let lo = 0;
  let hi = layout.tops.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (layout.tops[mid] <= y) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

/** The page the reader is "on": the one crossing the reading line. */
export function pageAtOffset(layout: Layout, scrollTop: number, viewportHeight: number): number {
  const count = layout.tops.length;
  if (!count) return 1;
  if (scrollTop + viewportHeight >= layout.totalHeight - 2) return count;
  const line = scrollTop + viewportHeight * READING_LINE;
  const index = indexAt(layout, line);
  const bottom = layout.tops[index] + layout.heights[index];
  // In the gap below a page, the next page is the one being approached.
  if (line > bottom && index + 1 < count) return index + 2;
  return index + 1;
}

/** Inclusive 1-based [first, last] page range to mount for this viewport. */
export function renderRange(layout: Layout, scrollTop: number, viewportHeight: number, overscan: number): [number, number] {
  const count = layout.tops.length;
  if (!count) return [1, 0];
  const first = indexAt(layout, scrollTop);
  const last = indexAt(layout, scrollTop + viewportHeight);
  return [Math.max(1, first + 1 - overscan), Math.min(count, last + 1 + overscan)];
}

export function fitScale(mode: "fit-width" | "fit-page", page: PageSize, viewport: PageSize, padding: number): number {
  const widthScale = (viewport.width - padding * 2) / page.width;
  const heightScale = (viewport.height - padding * 2) / page.height;
  const scale = mode === "fit-width" ? widthScale : Math.min(widthScale, heightScale);
  if (!Number.isFinite(scale)) return 1;
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale));
}

export function resolveScale(mode: ZoomMode, page: PageSize | undefined, viewport: PageSize, padding: number): number {
  if (typeof mode === "number") return Math.min(MAX_SCALE, Math.max(MIN_SCALE, mode));
  if (!page || viewport.width <= 0) return 1;
  return fitScale(mode, page, viewport, padding);
}

export function stepZoom(current: number, direction: 1 | -1): number {
  if (direction > 0) return ZOOM_STEPS.find((step) => step > current + 0.001) ?? ZOOM_STEPS[ZOOM_STEPS.length - 1];
  return [...ZOOM_STEPS].reverse().find((step) => step < current - 0.001) ?? ZOOM_STEPS[0];
}

export interface ScrollAnchor {
  page: number;
  /** How far down the page (0..1) the top edge of the viewport sits. */
  fraction: number;
}

export function captureAnchor(layout: Layout, scrollTop: number): ScrollAnchor {
  if (!layout.tops.length) return { page: 1, fraction: 0 };
  const index = indexAt(layout, scrollTop);
  const height = layout.heights[index] || 1;
  const fraction = Math.min(1, Math.max(0, (scrollTop - layout.tops[index]) / height));
  return { page: index + 1, fraction };
}

export function scrollTopForAnchor(layout: Layout, anchor: ScrollAnchor): number {
  const index = Math.min(layout.tops.length - 1, Math.max(0, anchor.page - 1));
  if (index < 0) return 0;
  return layout.tops[index] + anchor.fraction * layout.heights[index];
}

export function scrollTopForPage(layout: Layout, page: number): number {
  const index = Math.min(layout.tops.length - 1, Math.max(0, page - 1));
  if (index <= 0) return 0;
  return Math.max(0, layout.tops[index] - JUMP_MARGIN);
}

/** scrollTop that puts a normalised y (0..1) of a page at ~30% of the viewport. */
export function scrollTopForPoint(layout: Layout, page: number, y: number, viewportHeight: number): number {
  const index = Math.min(layout.tops.length - 1, Math.max(0, page - 1));
  if (index < 0) return 0;
  return Math.max(0, layout.tops[index] + y * layout.heights[index] - viewportHeight * 0.3);
}

export type KeyAction = "next" | "prev" | "first" | "last" | "find" | "zoomIn" | "zoomOut" | "zoomReset";

export interface KeyInput {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  /** True when focus is in an input/textarea/contenteditable. */
  inEditable: boolean;
}

export function keyAction(input: KeyInput): KeyAction | null {
  const mod = input.ctrlKey || input.metaKey;
  if (mod && !input.altKey) {
    const key = input.key.toLowerCase();
    if (key === "f") return "find";
    if (input.inEditable) return null;
    if (key === "=" || key === "+") return "zoomIn";
    if (key === "-" || key === "_") return "zoomOut";
    if (key === "0") return "zoomReset";
    return null;
  }
  if (input.inEditable || input.altKey || mod) return null;
  switch (input.key) {
    case "ArrowRight":
    case "PageDown":
      return "next";
    case "ArrowLeft":
    case "PageUp":
      return "prev";
    case "Home":
      return "first";
    case "End":
      return "last";
    default:
      return null;
  }
}
