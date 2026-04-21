// page.tsx — Reader view (hero view).
// Three-column layout: Sidebar · Doc pane · Matches panel.
// This is the core view where users read PDFs and review semantic suggestions.

"use client";

import { useState, useEffect } from "react";
import { Sidebar } from "@/components/Sidebar";
import { Icon } from "@/components/ui/Icon";
import { clientFetch, type LinkSuggestion } from "@/lib/api";
import { useSelar } from "@/lib/context";

const RELATION_LABELS: Record<string, string> = {
  related_to: "related to",
  prerequisite_of: "prerequisite of",
  sub_concept_of: "sub-concept of",
  contradicts: "contradicts",
  extends: "extends",
};

export default function ReaderPage() {
  const { user } = useSelar();
  const [docId, setDocId] = useState("backprop");
  
  const [suggestions, setSuggestions] = useState<LinkSuggestion[]>([]);
  const [loading, setLoading] = useState(false);
  const [zoom, setZoom] = useState(1);
  const [annotationsOn, setAnnotationsOn] = useState(true);

  // Fetch suggestions when docId changes
  useEffect(() => {
    if (!docId) return;
    setLoading(true);
    // 0 fetches all pages for now
    clientFetch<LinkSuggestion[]>(`/api/documents/${docId}/suggestions?page=0`)
      .then((data) => {
        setSuggestions(data || []);
      })
      .catch((err) => {
        console.error("Failed to fetch suggestions", err);
        setSuggestions([]);
      })
      .finally(() => {
        setLoading(false);
      });
  }, [docId]);

  // Handle responding to a suggestion
  const respond = async (id: string, action: "confirmed" | "rejected" | "relabeled") => {
    try {
      // Optimistic update
      setSuggestions(prev => prev.map(s => 
        s.id === id ? { ...s, status: action } : s
      ));

      await clientFetch(`/api/suggestions/${id}/respond`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, time_to_respond_ms: 1500 }), // Mocked 1.5s response time
      });
    } catch (err) {
      console.error("Failed to submit response", err);
      // Ideally revert optimistic update here, skipped for brevity
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
            <button>‹</button>
            <span className="page-indicator">4 / 14</span>
            <button>›</button>
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

        <div
          style={{
            transform: `scale(${zoom})`,
            transformOrigin: "top center",
            transition: "transform 0.12s",
          }}
        >
          {/* PDF Page mock — will be replaced by react-pdf canvas + overlay layer */}
          <div className="pdf-page">
            <div
              style={{
                marginBottom: 16,
                fontFamily: "var(--font-mono)",
                fontSize: 10,
                color: "#aaa",
                display: "flex",
                justifyContent: "space-between",
              }}
            >
              <span>{docId}</span>
              <span>page 4</span>
            </div>
            
            <div style={{ textAlign: "center", padding: 40, color: "var(--ink-4)" }}>
              <p>Database is empty.</p>
              <p style={{ fontSize: "var(--t-sm)" }}>Waiting for Python ingest script to populate PDF chunks and execute AI matching.</p>
            </div>
          </div>
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
               No suggestions generated yet for this document.
             </div>
          )}

          {suggestions.map((m) => {
            const st = m.status;
            return (
              <div
                key={m.id}
                className={`match-card${
                  st === "confirmed" ? " confirmed" : ""
                }${st === "rejected" ? " rejected" : ""}`}
              >
                <div className="row1">
                  <span className="sim">{(m.similarity * 100).toFixed(0)}%</span>
                  <div className="sim-bar">
                    <div
                      className="fill"
                      style={{ width: `${m.similarity * 100}%` }}
                    />
                  </div>
                  <span className="src">{m.tgt_doc || "Unknown Document"}</span>
                </div>
                <div className="snippet">{m.tgt_text}</div>
                <div className="foot">
                  {st === "confirmed" ? (
                    <span
                      style={{
                        fontFamily: "var(--font-mono)",
                        fontSize: 10,
                        color: "var(--accent-2)",
                      }}
                    >
                      <Icon name="check" size={11} /> confirmed ·{" "}
                      {RELATION_LABELS[m.relation] || m.relation}
                    </span>
                  ) : st === "rejected" ? (
                    <span
                      style={{
                        fontFamily: "var(--font-mono)",
                        fontSize: 10,
                        color: "var(--ink-4)",
                      }}
                    >
                      <Icon name="x" size={11} /> rejected
                    </span>
                  ) : (
                    <>
                      <button
                        className="confirm"
                        onClick={() => respond(m.id, "confirmed")}
                      >
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
                  <span
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 10,
                      color: "var(--ink-4)",
                    }}
                  >
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
