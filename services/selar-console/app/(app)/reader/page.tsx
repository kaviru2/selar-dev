// page.tsx — Reader view (hero view).
// Three-column layout: Sidebar · Doc pane · Matches panel.
// Now incorporates react-pdf for true PDF rendering.

"use client";

import { useState, useEffect } from "react";
import { Sidebar } from "@/components/Sidebar";
import { Icon } from "@/components/ui/Icon";
import { clientFetch, type LinkSuggestion } from "@/lib/api";
import { useSelar } from "@/lib/context";
import dynamic from "next/dynamic";

const PdfCanvas = dynamic(() => import("@/components/PdfCanvas"), {
  ssr: false,
  loading: () => <div style={{ padding: 40, fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--ink-4)" }}>Initializing PDF engine...</div> 
});

const RELATION_LABELS: Record<string, string> = {
  related_to: "related to",
  prerequisite_of: "prerequisite of",
  sub_concept_of: "sub-concept of",
  contradicts: "contradicts",
  extends: "extends",
};

import { useSearchParams } from 'next/navigation';

export default function ReaderPage() {
  const { user } = useSelar();
  const searchParams = useSearchParams();
  const urlDocId = searchParams.get('docId') || "";
  
  const [docId, setDocId] = useState(urlDocId);
  const [suggestions, setSuggestions] = useState<LinkSuggestion[]>([]);
  const [annotations, setAnnotations] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [zoom, setZoom] = useState(1);
  const [annotationsOn, setAnnotationsOn] = useState(true);

  const [numPages, setNumPages] = useState<number>(0);
  const [pageNumber, setPageNumber] = useState<number>(1);

  // Fetch suggestions when docId changes
  useEffect(() => {
    if (!docId) return;
    setLoading(true);
    // 0 fetches all pages for now
    Promise.all([
      clientFetch<LinkSuggestion[]>(`/api/documents/${docId}/suggestions?page=0`),
      clientFetch<any[]>(`/api/documents/${docId}/annotations`)
    ])
      .then(([sData, aData]) => {
        setSuggestions(sData || []);
        setAnnotations(aData || []);
      })
      .catch((err) => {
        console.error("Failed to fetch suggestions", err);
        setSuggestions([]);
        setAnnotations([]);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [docId]);

  const handleCreateAnnotation = async (type: string, bboxes: any[], color: string, pageIndex: number, comment: string = "") => {
    try {
      const resp = await clientFetch<any>(`/api/annotations`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          document_id: docId,
          page: pageIndex,
          bbox: JSON.stringify(bboxes),
          type,
          color,
          comment
        })
      });
      if (resp && resp.id) {
         setAnnotations(prev => [...prev, resp]);
      }
    } catch (err) {
      console.error("Failed to save annotation", err);
    }
  };

  // Handle responding to a suggestion
  const respond = async (id: string, action: "confirmed" | "rejected" | "relabeled") => {
    try {
      setSuggestions(prev => prev.map(s => 
        s.id === id ? { ...s, status: action } : s
      ));

      await clientFetch(`/api/suggestions/${id}/respond`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, time_to_respond_ms: 1500 }),
      });
    } catch (err) {
      console.error("Failed to submit response", err);
    }
  };

  const pendingCount = suggestions.filter((m) => m.status === "pending").length;
  const confirmedCount = suggestions.filter((m) => m.status === "confirmed").length;

  return (
    <div className="reader">
      <Sidebar currentId={docId} onPick={setDocId} />

      {/* Doc pane */}
      <div className="doc-pane">
        <div className="doc-toolbar">
          <div className="grp">
            <button 
              disabled={pageNumber <= 1}
              onClick={() => setPageNumber(p => Math.max(1, p - 1))}
            >‹</button>
            <span className="page-indicator">{pageNumber} / {numPages || "?"}</span>
            <button 
              disabled={numPages === 0 || pageNumber >= numPages}
              onClick={() => setPageNumber(p => Math.min(numPages, p + 1))}
            >›</button>
          </div>
          <div className="grp">
            <button onClick={() => setZoom(Math.max(0.5, zoom - 0.1))}>
              <Icon name="zoom_out" size={12} />
            </button>
            <span className="page-indicator" style={{ padding: "0 6px" }}>
              {Math.round(zoom * 100)}%
            </span>
            <button onClick={() => setZoom(Math.min(2, zoom + 0.1))}>
              <Icon name="zoom_in" size={12} />
            </button>
          </div>
          <div className="grp">
            <button
              className={annotationsOn ? "on" : ""}
              onClick={() => setAnnotationsOn(!annotationsOn)}
            >
              <Icon name="highlight" size={12} /> Marks
            </button>
          </div>
          <div className="tool-spacer" />
          <div className="grp">
            <button><Icon name="search" size={12} /></button>
            <button><Icon name="note" size={12} /> Note</button>
            <button><Icon name="more" size={12} /></button>
          </div>
        </div>

        <div className="pdf-container" style={{
            flex: 1,
            overflow: "auto",
            display: "flex",
            justifyContent: "center",
            padding: "20px 0",
            background: "var(--bg-2)"
        }}>
          <PdfCanvas 
            docId={docId} 
            zoom={zoom} 
            pageNumber={pageNumber} 
            annotationsOn={annotationsOn} 
            suggestions={suggestions}
            annotations={annotations}
            onCreateAnnotation={handleCreateAnnotation}
            onPageLoad={setNumPages} 
          />
        </div>
      </div>

      {/* Matches panel */}
      <aside className="matches">
        <div className="matches-head">
          <span className="ttl">Matches</span>
          <span className="chip">{pendingCount} pending</span>
          <span
            className="chip"
            style={{
              color: "var(--accent-2)",
              borderColor: "rgba(122,140,92,0.4)",
            }}
          >
            {confirmedCount} linked
          </span>
          <div style={{ flex: 1 }} />
          <div className="seg" style={{
            display: "flex",
            background: "var(--bg-2)",
            borderRadius: "var(--r-sm)",
            padding: 2,
            fontFamily: "var(--font-mono)",
            fontSize: 10,
          }}>
            {["cards", "diff", "feed", "keyboard"].map((v) => (
              <button
                key={v}
                className={v === "cards" ? "on" : ""}
                style={{
                  background: v === "cards" ? "var(--bg)" : "transparent",
                  border: "none",
                  padding: "2px 7px",
                  borderRadius: 2,
                  color: v === "cards" ? "var(--ink)" : "var(--ink-3)",
                  cursor: "pointer",
                  fontFamily: "inherit",
                  boxShadow: v === "cards" ? "var(--shadow-1)" : "none",
                }}
              >
                {v}
              </button>
            ))}
          </div>
        </div>

        <div className="matches-body">
          <div className="match-group-lbl">
            Pending · {loading ? "..." : pendingCount}
          </div>
          
          {suggestions.length === 0 && !loading && (
             <div style={{ padding: 20, textAlign: "center", color: "var(--ink-4)", fontSize: "var(--t-sm)" }}>
               No suggestions generated yet for this page.
             </div>
          )}

          {suggestions.map((m) => {
            const st = m.status;
            const relColor: Record<string, string> = {
              prerequisite_of: "var(--accent)",
              extends: "var(--accent-2)",
              sub_concept_of: "var(--accent-3)",
              contradicts: "#c0443a",
              related_to: "var(--ink-4)",
            };
            const rc = relColor[m.relation] || "var(--ink-4)";

            return (
              <div
                key={m.id}
                className={`match-card${
                  st === "confirmed" ? " confirmed" : ""
                }${st === "rejected" ? " rejected" : ""}`}
              >
                {/* Row 1: Relation pill + similarity + target doc */}
                <div className="row1">
                  <span style={{
                    fontSize: 9, fontFamily: "var(--font-mono)", fontWeight: 600,
                    color: rc, border: `1px solid ${rc}`, borderRadius: 10,
                    padding: "1px 7px", textTransform: "uppercase" as const, letterSpacing: "0.04em",
                    whiteSpace: "nowrap",
                  }}>
                    {RELATION_LABELS[m.relation] || m.relation}
                  </span>
                  <span className="sim">{(m.similarity * 100).toFixed(0)}%</span>
                  <div className="sim-bar">
                    <div className="fill" style={{ width: `${m.similarity * 100}%` }} />
                  </div>
                  <span className="src">{m.tgt_doc || "Unknown"}</span>
                </div>

                {/* Row 2: AI Summary (if available) or truncated text */}
                {m.summary ? (
                  <div style={{
                    fontSize: 12, color: "var(--ink-2)", lineHeight: 1.45,
                    margin: "6px 0 8px", fontStyle: "italic",
                  }}>
                    {m.summary}
                  </div>
                ) : (
                  <div className="snippet">{m.tgt_text}</div>
                )}

                {/* Footer: actions + page ref */}
                <div className="foot">
                  {st === "confirmed" ? (
                    <span style={{
                      fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--accent-2)",
                    }}>
                      <Icon name="check" size={11} /> confirmed
                    </span>
                  ) : st === "rejected" ? (
                    <span style={{
                      fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)",
                    }}>
                      <Icon name="x" size={11} /> rejected
                    </span>
                  ) : (
                    <>
                      <button className="confirm" onClick={() => respond(m.id, "confirmed")}>
                        <Icon name="check" size={11} /> Confirm
                      </button>
                      <button onClick={() => respond(m.id, "rejected")}>
                        <Icon name="x" size={11} /> Reject
                      </button>
                      <button>
                        <Icon name="tag" size={11} /> Label
                      </button>
                    </>
                  )}
                  <div style={{ flex: 1 }} />
                  <span style={{
                    fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)",
                  }}>
                    p.{m.tgt_page}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      </aside>
    </div>
  );
}
