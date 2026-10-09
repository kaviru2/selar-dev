import type { Annotation } from "@/lib/api";
import { makeTextAnchor, resolveTextAnchor, type TextAnchor } from "./annotation-anchor";
import { mergeLineRects, normalizeRects, parseBBoxes, type NormalizedBBox } from "./rects";

export function annotationSegments(page: Element): HTMLElement[] {
  return Array.from(page.querySelectorAll<HTMLElement>(".textLayer span")).filter(span => !span.classList.contains("markedContent") && !span.querySelector("span"));
}

/** Capture only a single page's text layer. Never include toolbar or adjacent-page text. */
export function captureSelectionAnchor(page: Element, range: Range, hash: string): TextAnchor | null {
  const spans = annotationSegments(page);
  const offset = (node: Node, at: number) => {
    let total = 0;
    for (const span of spans) {
      if (span.contains(node)) {
        const prefix = document.createRange(); prefix.selectNodeContents(span); prefix.setEnd(node, at);
        return total + prefix.toString().length;
      }
      total += span.textContent?.length || 0;
    }
    return -1;
  };
  return makeTextAnchor(spans.map(s => s.textContent || "").join(""), offset(range.startContainer, range.startOffset), offset(range.endContainer, range.endOffset), hash);
}

// Find a DOM point even while the find tool has wrapped part of a span in <mark>.
function point(span: HTMLElement, offset: number): [Node, number] | null {
  const walker = document.createTreeWalker(span, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const length = node.textContent?.length || 0;
    if (offset <= length) return [node, offset];
    offset -= length;
  }
  return null;
}

export function annotationBoxes(page: Element | null, a: Annotation, hash: string): NormalizedBBox[] {
  if (!a.anchor) return parseBBoxes(a.bbox);
  if (!hash || a.anchor.source_hash !== hash) return [];
  if (page) {
    const spans = annotationSegments(page);
    const match = resolveTextAnchor(spans.map(s => s.textContent || ""), a.anchor, hash);
    if (match) {
      const start = point(spans[match.start.segment], match.start.offset), end = point(spans[match.end.segment], match.end.offset);
      if (start && end) {
        const range = document.createRange(); range.setStart(...start); range.setEnd(...end);
        const boxes = normalizeRects(mergeLineRects(Array.from(range.getClientRects())), page.getBoundingClientRect());
        if (boxes.length) return boxes;
      }
    }
  }
  // Same immutable source only. These are normalised, not current screen pixels.
  return parseBBoxes(a.bbox);
}
