// pdfCoords.ts — PDF-space ↔ screen-space coordinate converter.
// All SELAR annotations and chunk bboxes are stored in PDF points
// (1/72 inch, origin at bottom-left). This module handles the conversion
// to/from CSS pixels for rendering overlays on the pdf.js canvas.
// Every component that touches coordinates must go through these two functions.

export interface PdfBbox {
  x: number;
  y: number;
  w: number;
  h: number;
  unit: "pdf_points";
}

export interface ScreenRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

/**
 * PageViewport is a simplified interface matching pdf.js PageViewport.
 * We define it here to avoid importing the full pdfjs-dist types.
 */
export interface PageViewport {
  convertToViewportPoint(x: number, y: number): [number, number];
  convertToPdfPoint(x: number, y: number): [number, number];
}

/**
 * Convert a PDF-space bounding box to CSS pixel coordinates for overlay rendering.
 * Handles the Y-axis flip (PDF origin is bottom-left, CSS is top-left).
 */
export function pdfToScreen(bbox: PdfBbox, viewport: PageViewport): ScreenRect {
  // Bottom-left + top-right of the bbox in PDF space
  const [x1, y1] = viewport.convertToViewportPoint(bbox.x, bbox.y + bbox.h);
  const [x2, y2] = viewport.convertToViewportPoint(bbox.x + bbox.w, bbox.y);
  return {
    left: Math.min(x1, x2),
    top: Math.min(y1, y2),
    width: Math.abs(x2 - x1),
    height: Math.abs(y2 - y1),
  };
}

/**
 * Convert a screen-space rectangle back to PDF-space bounding box.
 * Used when the user creates a new annotation by selecting text.
 */
export function screenToPdf(
  rect: { left: number; top: number; width: number; height: number },
  viewport: PageViewport
): PdfBbox {
  const [x1, y1] = viewport.convertToPdfPoint(
    rect.left,
    rect.top + rect.height
  );
  const [x2, y2] = viewport.convertToPdfPoint(
    rect.left + rect.width,
    rect.top
  );
  return {
    x: Math.min(x1, x2),
    y: Math.min(y1, y2),
    w: Math.abs(x2 - x1),
    h: Math.abs(y2 - y1),
    unit: "pdf_points",
  };
}
