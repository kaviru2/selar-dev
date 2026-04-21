// Sidebar.tsx — SELAR library sidebar.
// Displays the list of documents in the user's corpus with status indicators,
// search with ⌘K hint, and Drive sync footer.
// Matches design_handoff_selar §3 sidebar spec — 240px wide, compact items.

"use client";

import { Icon } from "@/components/ui/Icon";
import { useEffect, useState } from "react";
import { clientFetch } from "@/lib/api";

interface SidebarDoc {
  id: string;
  title: string;
  authors: string;
  year: number;
  status: "ready" | "processing" | "failed";
  chunks_count: number;
  links_count: number;
  page_count: number;
  progress?: number;
}

interface SidebarProps {
  currentId?: string;
  onPick?: (id: string) => void;
}

export function Sidebar({ currentId, onPick }: SidebarProps) {
  const [docs, setDocs] = useState<SidebarDoc[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    clientFetch<SidebarDoc[]>("/api/documents")
      .then((data) => {
        setDocs(data || []);
        // If currentId is falsy or 'backprop', auto-select the first real doc
        if (data && data.length > 0 && (!currentId || currentId === "backprop")) {
          onPick?.(data[0].id);
        }
      })
      .catch((err) => console.error("Failed to fetch sidebar docs:", err))
      .finally(() => setLoading(false));
  }, [currentId, onPick]);

  return (
    <aside className="sidebar">
      <div className="sb-head">
        <span className="lbl">Library</span>
        <span className="count">{docs.length}</span>
        <button
          title="Upload"
          onClick={() => window.location.href = '/library'}
          style={{
            marginLeft: 6,
            background: "transparent",
            border: "1px solid var(--rule)",
            borderRadius: 3,
            width: 20,
            height: 20,
            display: "grid",
            placeItems: "center",
            cursor: "pointer",
            color: "var(--ink-3)",
          }}
        >
          <Icon name="plus" size={10} />
        </button>
      </div>

      <div className="sb-search">
        <Icon name="search" size={11} style={{ color: "var(--ink-4)" }} />
        <input placeholder="Filter…" />
        <span className="kbd">⌘K</span>
      </div>

      <div className="sb-group">Your Documents</div>

      <div style={{ paddingBottom: 8, overflow: "auto", flex: 1 }}>
        {loading && <div style={{ padding: 12, fontSize: 12, color: "var(--ink-4)" }}>Loading corpus...</div>}
        {!loading && docs.length === 0 && (
          <div style={{ padding: 12, fontSize: 12, color: "var(--ink-4)" }}>No documents uploaded.</div>
        )}
        {docs.map((d) => {
          const authorShort = d.authors ? d.authors.split(" ")[0] : "Unknown";
          const hasEtAl = d.authors && d.authors.includes("et al.");
          const metaText =
            d.status === "processing"
              ? `processing`
              : d.status === "failed"
              ? "failed"
              : `${d.links_count} links`;

          return (
            <div
              key={d.id}
              className={`sb-item${currentId === d.id ? " active" : ""}`}
              onClick={() => onPick?.(d.id)}
            >
              <span className={`dot ${d.status}`} />
              <span className="title" style={{ flex: 1 }}>
                <span className="t">{d.title}</span>
                <span className="meta">
                  {authorShort}
                  {hasEtAl ? " et al." : ""} · {d.year || "N/A"} · {metaText}
                </span>
              </span>
              <button 
                title="Delete document"
                onClick={async (e) => {
                  e.stopPropagation();
                  if (!confirm("Delete this document and all its indexed chunks?")) return;
                  try {
                    setLoading(true);
                    await clientFetch(`/api/documents/${d.id}`, { method: 'DELETE' });
                    // Refresh docs list
                    const remaining = docs.filter(doc => doc.id !== d.id);
                    setDocs(remaining);
                    if (currentId === d.id && remaining.length > 0) {
                       onPick?.(remaining[0].id);
                    }
                  } catch (err) {
                    console.error("Failed to delete", err);
                  } finally {
                    setLoading(false);
                  }
                }}
                style={{
                  background: "transparent",
                  border: "none",
                  cursor: "pointer",
                  color: "var(--ink-4)",
                  padding: "4px",
                  display: "grid",
                  placeItems: "center",
                  borderRadius: "3px"
                }}
                onMouseEnter={e => e.currentTarget.style.color = "var(--error)"}
                onMouseLeave={e => e.currentTarget.style.color = "var(--ink-4)"}
              >
                <Icon name="trash" size={12} />
              </button>
            </div>
          );
        })}
      </div>

      <div className="sb-foot">
        <Icon name="drive" size={12} style={{ color: "var(--accent-2)" }} />
        <span style={{ fontFamily: "var(--font-mono)", fontSize: 10 }}>
          Drive · connected
        </span>
      </div>
    </aside>
  );
}
