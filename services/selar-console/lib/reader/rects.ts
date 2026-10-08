// rects.ts — turn DOM Range client rects into clean, normalised page boxes.

export interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

export interface NormalizedBBox {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Merge per-span rects into one box per visual line, top to bottom. */
export function mergeLineRects(rects: Box[]): Box[] {
  const usable = rects
    .filter((rect) => rect.width >= 1 && rect.height >= 1)
    .map((rect) => ({ left: rect.left, top: rect.top, right: rect.left + rect.width, bottom: rect.top + rect.height }));
  // A rect that spans several other rects vertically is a container artefact.
  const filtered = usable.filter((rect) => {
    const height = rect.bottom - rect.top;
    const inside = usable.filter((other) => other !== rect && other.top >= rect.top - 1 && other.bottom <= rect.bottom + 1 && other.bottom - other.top < height * 0.6);
    return inside.length < 2;
  });
  // Left-to-right so each rect only needs to touch the growing right edge.
  filtered.sort((a, b) => a.left - b.left || a.top - b.top);

  const lines: Array<{ left: number; top: number; right: number; bottom: number }> = [];
  for (const rect of filtered) {
    const height = rect.bottom - rect.top;
    const line = lines.find((candidate) => {
      const overlap = Math.min(candidate.bottom, rect.bottom) - Math.max(candidate.top, rect.top);
      const sameLine = overlap > Math.min(height, candidate.bottom - candidate.top) * 0.5;
      const touching = rect.left <= candidate.right + height * 0.6 && rect.right >= candidate.left - height * 0.6;
      return sameLine && touching;
    });
    if (line) {
      line.left = Math.min(line.left, rect.left);
      line.right = Math.max(line.right, rect.right);
      line.top = Math.min(line.top, rect.top);
      line.bottom = Math.max(line.bottom, rect.bottom);
    } else {
      lines.push({ ...rect });
    }
  }
  return lines
    .sort((a, b) => a.top - b.top || a.left - b.left)
    .map((line) => ({ left: line.left, top: line.top, width: line.right - line.left, height: line.bottom - line.top }));
}

const round = (value: number) => Math.round(value * 1e5) / 1e5;

export function normalizeRects(rects: Box[], page: Box): NormalizedBBox[] {
  if (page.width <= 0 || page.height <= 0) return [];
  return rects.map((rect) => {
    const x1 = Math.min(1, Math.max(0, (rect.left - page.left) / page.width));
    const y1 = Math.min(1, Math.max(0, (rect.top - page.top) / page.height));
    const x2 = Math.min(1, Math.max(0, (rect.left + rect.width - page.left) / page.width));
    const y2 = Math.min(1, Math.max(0, (rect.top + rect.height - page.top) / page.height));
    return { x: round(x1), y: round(y1), w: round(x2 - x1), h: round(y2 - y1) };
  }).filter((box) => box.w > 0 && box.h > 0);
}

export function parseBBoxes(value: unknown): NormalizedBBox[] {
  try {
    const parsed: unknown = typeof value === "string" ? JSON.parse(value) : value;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((item): item is NormalizedBBox => {
      if (!item || typeof item !== "object") return false;
      const box = item as Record<string, unknown>;
      return ["x", "y", "w", "h"].every((key) => typeof box[key] === "number" && Number.isFinite(box[key]));
    });
  } catch {
    return [];
  }
}
