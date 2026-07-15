"use client";

import { useEffect, useRef, useState } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import type { Annotation, LinkSuggestion } from "@/lib/api";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`;

interface NormalizedBBox {
  x: number;
  y: number;
  w: number;
  h: number;
}

interface SelectionMenu {
  x: number;
  y: number;
  bboxes: NormalizedBBox[];
}

interface PdfCanvasProps {
  docId: string;
  zoom: number;
  pageNumber: number;
  annotationsOn: boolean;
  suggestions: LinkSuggestion[];
  annotations?: Annotation[];
  onCreateAnnotation?: (
    type: string,
    bboxes: NormalizedBBox[],
    color: string,
    pageIndex: number,
    comment?: string
  ) => void;
  onPageLoad: (numPages: number) => void;
}

function parseBBoxes(value: Annotation["bbox"] | LinkSuggestion["src_bboxes"]): NormalizedBBox[] {
  try {
    const parsed: unknown = typeof value === "string" ? JSON.parse(value) : value;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((item): item is NormalizedBBox => {
      if (!item || typeof item !== "object") return false;
      const box = item as Record<string, unknown>;
      return ["x", "y", "w", "h"].every((key) => typeof box[key] === "number");
    });
  } catch {
    return [];
  }
}

export default function PdfCanvas({
  docId,
  zoom,
  pageNumber,
  annotationsOn,
  suggestions,
  annotations = [],
  onCreateAnnotation,
  onPageLoad,
}: PdfCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const pageRef = useRef<HTMLDivElement>(null);
  const [selectionMenu, setSelectionMenu] = useState<SelectionMenu | null>(null);

  useEffect(() => {
    function handleSelection() {
      const selection = window.getSelection();
      const container = containerRef.current;
      const page = pageRef.current;
      if (!selection || selection.isCollapsed || !container || !page || !selection.rangeCount) {
        setSelectionMenu(null);
        return;
      }

      const range = selection.getRangeAt(0);
      if (!page.contains(range.commonAncestorContainer)) return;
      const pageRect = page.getBoundingClientRect();
      const containerRect = container.getBoundingClientRect();
      const rects = Array.from(range.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0);
      if (!rects.length || !pageRect.width || !pageRect.height) return;

      const bboxes = rects.map((rect) => ({
        x: (rect.left - pageRect.left) / pageRect.width,
        y: (rect.top - pageRect.top) / pageRect.height,
        w: rect.width / pageRect.width,
        h: rect.height / pageRect.height,
      }));
      const lastRect = rects[rects.length - 1];
      setSelectionMenu({
        x: Math.min(lastRect.right - containerRect.left + 6, containerRect.width - 150),
        y: lastRect.top - containerRect.top,
        bboxes,
      });
    }

    document.addEventListener("mouseup", handleSelection);
    return () => document.removeEventListener("mouseup", handleSelection);
  }, [pageNumber, zoom]);

  function createSelection(type: "highlight" | "note") {
    if (!selectionMenu || !onCreateAnnotation) return;
    if (type === "highlight") {
      onCreateAnnotation(type, selectionMenu.bboxes, "yellow", pageNumber);
    } else {
      const comment = window.prompt("Add a note about this passage:");
      if (comment?.trim()) {
        onCreateAnnotation(type, selectionMenu.bboxes, "sage", pageNumber, comment.trim());
      }
    }
    window.getSelection()?.removeAllRanges();
    setSelectionMenu(null);
  }

  const pageAnnotations = annotations.filter((annotation) => annotation.page === pageNumber);
  const renderedSuggestionBoxes = new Set<string>();

  return (
    <div ref={containerRef} className="pdf-canvas-shell">
      <Document
        file={`/api/documents/${docId}/pdf`}
        onLoadSuccess={({ numPages }) => onPageLoad(numPages)}
        loading={<div className="reader-empty">Loading document…</div>}
        error={<div className="reader-empty"><strong>Document unavailable</strong><span>Upload or reprocess the PDF to continue.</span></div>}
      >
        <div ref={pageRef} className="pdf-page-container">
          <Page
            pageNumber={pageNumber}
            scale={zoom}
            className="pdf-page-shadow"
            renderTextLayer
            renderAnnotationLayer
          />

          {annotationsOn && (
            <div className="pdf-overlay" aria-label="Document annotations">
              {pageAnnotations.flatMap((annotation) =>
                parseBBoxes(annotation.bbox).map((bbox, index) => (
                  <div
                    key={`annotation-${annotation.id}-${index}`}
                    className={`pdf-mark user-mark ${annotation.type}`}
                    style={{
                      left: `${bbox.x * 100}%`,
                      top: `${bbox.y * 100}%`,
                      width: `${bbox.w * 100}%`,
                      height: `${bbox.h * 100}%`,
                    }}
                    title={annotation.comment}
                  />
                ))
              )}

              {suggestions.flatMap((suggestion) => {
                if (suggestion.status === "rejected" || suggestion.src_page !== pageNumber) return [];
                return parseBBoxes(suggestion.src_bboxes)
                  .filter((bbox) => bbox.w * bbox.h <= 0.20)
                  .flatMap((bbox, index) => {
                  const key = `${suggestion.status}-${bbox.x.toFixed(4)}-${bbox.y.toFixed(4)}-${bbox.w.toFixed(4)}-${bbox.h.toFixed(4)}`;
                  if (renderedSuggestionBoxes.has(key)) return [];
                  renderedSuggestionBoxes.add(key);
                  return (
                    <div
                      key={`suggestion-${suggestion.id}-${index}`}
                      className={`pdf-mark suggestion-mark ${suggestion.status}`}
                      style={{
                        left: `${bbox.x * 100}%`,
                        top: `${bbox.y * 100}%`,
                        width: `${bbox.w * 100}%`,
                        height: `${bbox.h * 100}%`,
                      }}
                      title={`Connection to ${suggestion.tgt_doc}`}
                    />
                  );
                  });
              })}
            </div>
          )}
        </div>
      </Document>

      {selectionMenu && (
        <div className="selection-menu" style={{ left: selectionMenu.x, top: selectionMenu.y }}>
          <button onClick={() => createSelection("highlight")}>Highlight</button>
          <button onClick={() => createSelection("note")}>Add note</button>
        </div>
      )}
    </div>
  );
}
