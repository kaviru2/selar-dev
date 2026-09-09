export interface NormalizedBBox {
  x: number;
  y: number;
  w: number;
  h: number;
}

interface HighlightSuggestion {
  id: string;
  status: string;
  src_page: number;
  src_bboxes: NormalizedBBox[] | string;
}

export interface VisibleSuggestionBox {
  suggestionID: string;
  bbox: NormalizedBBox;
}

function parseBBoxes(value: HighlightSuggestion["src_bboxes"]): NormalizedBBox[] {
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

function isVisibleBox(box: NormalizedBBox): boolean {
  return box.x >= 0 && box.y >= 0 && box.w > 0 && box.h > 0 && box.x + box.w <= 1 && box.y + box.h <= 1 && box.w * box.h <= 0.20;
}

export function visibleSuggestionBoxes(suggestions: HighlightSuggestion[], page: number): VisibleSuggestionBox[] {
  const seen = new Set<string>();
  const visible: VisibleSuggestionBox[] = [];

  for (const suggestion of suggestions) {
    if (suggestion.status === "rejected" || suggestion.src_page !== page) continue;
    for (const bbox of parseBBoxes(suggestion.src_bboxes)) {
      if (!isVisibleBox(bbox)) continue;
      const key = `${bbox.x.toFixed(4)}-${bbox.y.toFixed(4)}-${bbox.w.toFixed(4)}-${bbox.h.toFixed(4)}`;
      if (seen.has(key)) continue;
      seen.add(key);
      visible.push({ suggestionID: suggestion.id, bbox });
    }
  }

  return visible;
}
