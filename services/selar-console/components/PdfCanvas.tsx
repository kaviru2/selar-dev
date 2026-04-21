"use client";

import { useState, useRef, useEffect } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`;

interface PdfCanvasProps {
  docId: string;
  zoom: number;
  pageNumber: number;
  annotationsOn: boolean;
  suggestions: any[];
  annotations?: any[];
  onCreateAnnotation?: (type: string, bboxes: any[], color: string, pageIndex: number, comment?: string) => void;
  onPageLoad: (numPages: number) => void;
}

export default function PdfCanvas({ docId, zoom, pageNumber, annotationsOn, suggestions, annotations = [], onCreateAnnotation, onPageLoad }: PdfCanvasProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [selectionMenu, setSelectionMenu] = useState<{ x: number, y: number, bboxes: any[], pageIndex: number } | null>(null);
  const [numPages, setLocalNumPages] = useState<number>(0);

  useEffect(() => {
    const handleSelection = () => {
      const selection = window.getSelection();
      if (!selection || selection.isCollapsed || !containerRef.current) {
        setTimeout(() => setSelectionMenu(null), 100);
        return;
      }

      const range = selection.getRangeAt(0);
      const rects = range.getClientRects();
      if (rects.length === 0) return;

      // Find which page this selection belongs to by traversing DOM
      let pageNode = selection.anchorNode?.parentElement?.closest('.react-pdf__Page');
      if (!pageNode) return;
      const pageIndexAttr = pageNode.getAttribute('data-page-number');
      const pageIndex = pageIndexAttr ? parseInt(pageIndexAttr, 10) : 1;

      const pageRect = pageNode.getBoundingClientRect();
      const containerRect = containerRef.current.getBoundingClientRect();
      
      const bboxes = Array.from(rects).map(rect => ({
        x: (rect.left - pageRect.left) / pageRect.width,
        y: (rect.top - pageRect.top) / pageRect.height,
        w: rect.width / pageRect.width,
        h: rect.height / pageRect.height
      }));

      const lastRect = rects[rects.length - 1];
      
      setSelectionMenu({
        x: lastRect.right - containerRect.left + 5,
        y: lastRect.top - containerRect.top,
        bboxes,
        pageIndex
      });
    };

    document.addEventListener("mouseup", handleSelection);
    return () => document.removeEventListener("mouseup", handleSelection);
  }, []);

  function onDocumentLoadSuccess({ numPages }: { numPages: number }) {
    setLocalNumPages(numPages);
    onPageLoad(numPages);
  }

  const handleHighlightClick = () => {
    if (selectionMenu && onCreateAnnotation) {
      // Pass the specific pageIndex instead of the global ReaderPage pageNumber
      onCreateAnnotation("highlight", selectionMenu.bboxes, "yellow", selectionMenu.pageIndex);
    }
    setSelectionMenu(null);
  };

  const handleNoteClick = () => {
    if (selectionMenu && onCreateAnnotation) {
      const comment = window.prompt("Enter your note for this selection:");
      if (comment) {
        onCreateAnnotation("note", selectionMenu.bboxes, "sage", selectionMenu.pageIndex, comment);
      }
    }
    setSelectionMenu(null);
  };

  return (
    <div 
      ref={containerRef}
      style={{
        position: "relative",
        width: "100%",
        display: "flex",
        flexDirection: "column",
        alignItems: "center"
      }}
    >
      <Document
        file={`/api/documents/${docId}/pdf`}
        onLoadSuccess={onDocumentLoadSuccess}
        loading={<div style={{ padding: 40, fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--ink-4)" }}>Loading PDF stream...</div>}
        error={
          <div style={{ padding: 40, textAlign: "center", color: "var(--ink-4)" }}>
            <p>Document not available.</p>
            <p style={{ fontSize: "var(--t-sm)", marginTop: 8 }}>Use the upload feature to process a real PDF into the semantic pipeline.</p>
          </div>
        }
      >
        {Array.from(new Array(numPages), (el, index) => {
          const currentPageNum = index + 1;
          
          return (
            <div key={`page_${currentPageNum}`} style={{ position: "relative", marginBottom: 20 }} className="pdf-page-container">
              <Page 
                pageNumber={currentPageNum} 
                scale={zoom}
                className="pdf-page-shadow" 
                renderTextLayer={true}
                renderAnnotationLayer={true}
              />
              
              {/* Bounding Box Highlights Canvas per Page */}
              {annotationsOn && (
                 <div style={{ position: "absolute", top: 0, left: 0, right: 0, bottom: 0, pointerEvents: "none" }}>
                     {/* Manual User Annotations */}
                     {annotations.map((ann, idx) => {
                       // Ensure annotation targets this specific page
                       if (ann.page !== currentPageNum) return null;
                       
                       let bboxes = [];
                       try {
                         bboxes = typeof ann.bbox === "string" ? JSON.parse(ann.bbox) : ann.bbox;
                       } catch(e) {}
                       
                       return bboxes.map((bbox: any, bIdx: number) => (
                         <div 
                           key={`ann-${ann.id}-${bIdx}`}
                           style={{
                             position: "absolute", left: `${bbox.x * 100}%`, top: `${bbox.y * 100}%`, width: `${bbox.w * 100}%`, height: `${bbox.h * 100}%`,
                             backgroundColor: ann.color === "yellow" ? "rgba(255, 235, 59, 0.4)" : "rgba(122, 140, 92, 0.4)",
                             mixBlendMode: "multiply", borderRadius: "3px", pointerEvents: "auto", cursor: "pointer"
                           }}
                           title={ann.comment}
                         />
                       ));
                     })}

                     {/* AI Semantic Suggestions */}
                     {suggestions.map((sg, idx) => {
                       let bboxes = [];
                       try {
                         bboxes = typeof sg.src_bboxes === "string" ? JSON.parse(sg.src_bboxes) : sg.src_bboxes;
                       } catch(e) {}
                       
                       return bboxes.map((bbox: any, bIdx: number) => {
                         const left = bbox.x * 100;
                         const top = bbox.y * 100;
                         const width = bbox.w * 100;
                         const height = bbox.h * 100;
                         
                           const colorMap: Record<string, { bg: string, border: string }> = {
                             "pending": { bg: "rgba(238, 201, 107, 0.08)", border: "rgba(238, 201, 107, 0.8)" },
                             "confirmed": { bg: "rgba(122, 140, 92, 0.08)", border: "rgba(122, 140, 92, 0.8)" }
                           };
                           const colors = colorMap[sg.status] || colorMap["pending"];
                           if (sg.status === "rejected") return null;

                           return (
                             <div 
                               key={`${sg.id}-${bIdx}`}
                               style={{
                                 position: "absolute", left: `${left}%`, top: `${top}%`, width: `${width}%`, height: `22px`,
                                 backgroundColor: colors.bg, 
                                 borderLeft: `4px solid ${colors.border}`,
                                 borderRadius: "0px 3px 3px 0px", 
                                 cursor: "pointer", 
                                 pointerEvents: "auto",
                                 transition: "background-color 0.2s"
                               }}
                               onMouseOver={(e) => (e.currentTarget.style.backgroundColor = "rgba(71, 161, 255, 0.20)")}
                               onMouseOut={(e) => (e.currentTarget.style.backgroundColor = colors.bg)}
                               title={`Suggestion: ${sg.tgt_doc}`}
                             />
                           );
                       });
                     })}
                 </div>
              )}
            </div>
          );
        })}
      </Document>

      {/* Manual Selection Popup Menu */}
      {selectionMenu && (
        <div style={{
          position: "absolute", left: selectionMenu.x, top: selectionMenu.y,
          background: "var(--bg)", boxShadow: "var(--shadow-2)", border: "1px solid var(--border)",
          borderRadius: "var(--r-md)", padding: 4, display: "flex", gap: 4, zIndex: 100
        }}>
          <button 
            style={{ 
              background: "transparent", border: "none", fontSize: "11px", fontFamily: "var(--font-mono)", 
              color: "var(--ink-2)", cursor: "pointer", padding: "4px 8px", borderRadius: "var(--r-sm)" 
            }}
            onMouseOver={(e) => (e.currentTarget.style.background = "var(--bg-hover)")}
            onMouseOut={(e) => (e.currentTarget.style.background = "transparent")}
            onClick={handleHighlightClick}
          >
            Highlight
          </button>
          <button 
            style={{ 
              background: "transparent", border: "none", fontSize: "11px", fontFamily: "var(--font-mono)", 
              color: "var(--ink-2)", cursor: "pointer", padding: "4px 8px", borderRadius: "var(--r-sm)" 
            }}
            onMouseOver={(e) => (e.currentTarget.style.background = "var(--bg-hover)")}
            onMouseOut={(e) => (e.currentTarget.style.background = "transparent")}
            onClick={handleNoteClick}
          >
            Note
          </button>
        </div>
      )}
    </div>
  );
}
