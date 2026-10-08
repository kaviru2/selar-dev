"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import { visibleSuggestionBoxes } from "@/lib/reader-highlights";
import type { Annotation, LinkSuggestion } from "@/lib/api";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  "pdfjs-dist/build/pdf.worker.min.mjs",
  import.meta.url,
).toString();

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

interface SuggestionMenu {
  suggestionId: string;
  x: number;
  y: number;
}

interface PdfCanvasProps {
  docId: string;
  zoom: number;
  pageNumber: number;
  annotationsOn: boolean;
  suggestionsOn: boolean;
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
  onScrollDepth?: (depth: number) => void;
  // Passage matches are similarity-only (relation "unclassified"); the API
  // refuses to confirm them, so the reader can only dismiss them.
  onRespondSuggestion: (id: string, action: "rejected") => Promise<boolean>;
  onOpenSuggestionTarget: (suggestion: LinkSuggestion) => void;
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
  suggestionsOn,
  suggestions,
  annotations = [],
  onCreateAnnotation,
  onPageLoad,
  onScrollDepth,
  onRespondSuggestion,
  onOpenSuggestionTarget,
}: PdfCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const pageRef = useRef<HTMLDivElement>(null);
  const [selectionMenu, setSelectionMenu] = useState<SelectionMenu | null>(null);
  const [suggestionMenu, setSuggestionMenu] = useState<SuggestionMenu | null>(null);
  const [suggestionBusy, setSuggestionBusy] = useState(false);
  const [suggestionError, setSuggestionError] = useState("");

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
  const selectedSuggestion = suggestions.find((suggestion) => suggestion.id === suggestionMenu?.suggestionId);
  const suggestionByID = useMemo(() => new Map(suggestions.map((suggestion) => [suggestion.id, suggestion])), [suggestions]);
  const pageSuggestionBoxes = useMemo(() => visibleSuggestionBoxes(suggestions, pageNumber), [suggestions, pageNumber]);

  async function respondToSelectedSuggestion(action: "rejected") {
    if (!selectedSuggestion || suggestionBusy) return;
    setSuggestionBusy(true);
    setSuggestionError("");
    const saved = await onRespondSuggestion(selectedSuggestion.id, action);
    setSuggestionBusy(false);
    if (saved) setSuggestionMenu(null);
    else setSuggestionError("Could not save this response. Please retry.");
  }

  return (
    <div
      ref={containerRef}
      className="pdf-canvas-shell"
      onScroll={(event) => {
        const target = event.currentTarget;
        const scrollable = target.scrollHeight - target.clientHeight;
        onScrollDepth?.(scrollable > 0 ? (target.scrollTop / scrollable) * 100 : 0);
      }}
    >
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

          {(annotationsOn || suggestionsOn) && (
            <div className="pdf-overlay" aria-label="Document annotations">
              {annotationsOn && pageAnnotations.flatMap((annotation) =>
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

              {suggestionsOn && pageSuggestionBoxes.map(({ suggestionID, bbox }, index) => {
                const suggestion = suggestionByID.get(suggestionID);
                if (!suggestion) return null;
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
                    role="button"
                    tabIndex={0}
                    aria-label={`Suggested ${suggestion.relation.replaceAll("_", " ")} connection to ${suggestion.tgt_doc}, page ${suggestion.tgt_page}`}
                    onClick={(event) => {
                      event.stopPropagation();
                      const bounds = containerRef.current?.getBoundingClientRect();
                      if (!bounds) return;
                      setSuggestionError("");
                      setSuggestionMenu({
                        suggestionId: suggestion.id,
                        x: Math.min(Math.max(8, event.clientX - bounds.left + 8), Math.max(8, bounds.width - 292)),
                        y: Math.max(8, event.clientY - bounds.top + 8),
                      });
                    }}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") event.currentTarget.click();
                    }}
                  />
                );
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

      {suggestionMenu && selectedSuggestion && (
        <div className="suggestion-popover" style={{ left: suggestionMenu.x, top: suggestionMenu.y }} role="dialog" aria-label="Suggested connection">
          <div className="suggestion-popover-head">
            <span>{selectedSuggestion.relation.replaceAll("_", " ")}</span>
            <strong>{Math.round(selectedSuggestion.similarity * 100)}%</strong>
            <button aria-label="Close suggestion" onClick={() => setSuggestionMenu(null)}>×</button>
          </div>
          <h3>{selectedSuggestion.tgt_doc} · page {selectedSuggestion.tgt_page}</h3>
          <p>{selectedSuggestion.summary || selectedSuggestion.tgt_text.slice(0, 220)}</p>
          <p className="suggestion-popover-note">Similarity only: these passages use similar wording. This is not a confirmed relation, so it cannot be accepted into your graph.</p>
          {suggestionError && <div className="suggestion-popover-error">{suggestionError}</div>}
          <div className="suggestion-popover-actions">
            <button
              className="open-target"
              disabled={!selectedSuggestion.tgt_document_id || suggestionBusy}
              onClick={() => onOpenSuggestionTarget(selectedSuggestion)}
            >Open connected passage</button>
            {selectedSuggestion.status === "pending" ? (
              <button disabled={suggestionBusy} onClick={() => respondToSelectedSuggestion("rejected")}>× Dismiss</button>
            ) : <span className={`suggestion-popover-state ${selectedSuggestion.status}`}>{selectedSuggestion.status === "rejected" ? "dismissed" : selectedSuggestion.status}</span>}
          </div>
        </div>
      )}
    </div>
  );
}
