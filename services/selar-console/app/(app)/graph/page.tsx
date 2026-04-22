// page.tsx — Knowledge Graph view with force-directed layout.
// Uses react-force-graph-2d for interactive physics-based visualization.

"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { Icon } from "@/components/ui/Icon";
import { clientFetch, type Concept, type ConceptEdge } from "@/lib/api";
import dynamic from "next/dynamic";

const ForceGraph2D = dynamic(() => import("react-force-graph-2d"), { ssr: false });

interface GraphNode {
  id: string;
  name: string;
  description: string;
  created_at: string;
  val: number; // size
  color: string;
}
interface GraphLink {
  id: string;
  source: string;
  target: string;
  relation: string;
  created_via: string;
}

const REL_COLORS: Record<string, string> = {
  prerequisite_of: "#c96442",
  related_to: "#9a938a",
  sub_concept_of: "#8a6a3d",
  contradicts: "#c0443a",
  extends: "#7a8c5c",
};

const NODE_PALETTE = [
  "#c96442", "#7a8c5c", "#8a6a3d", "#5e8aaa", "#9a6a9a", "#6a9a8a", "#aa7a5a", "#5a7aaa",
];

export default function GraphPage() {
  const [nodes, setNodes] = useState<GraphNode[]>([]);
  const [links, setLinks] = useState<GraphLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [selId, setSelId] = useState<string | null>(null);
  const fgRef = useRef<any>(null);

  useEffect(() => {
    clientFetch<{ nodes: Concept[]; edges: ConceptEdge[] }>("/api/graph")
      .then((data) => {
        // Count connections per node
        const connCount: Record<string, number> = {};
        for (const e of data.edges) {
          connCount[e.source_concept_id] = (connCount[e.source_concept_id] || 0) + 1;
          connCount[e.target_concept_id] = (connCount[e.target_concept_id] || 0) + 1;
        }

        const gNodes: GraphNode[] = data.nodes.map((n, i) => ({
          id: n.id,
          name: n.name,
          description: n.description,
          created_at: n.created_at,
          val: Math.max(2, (connCount[n.id] || 0) + 1),
          color: NODE_PALETTE[i % NODE_PALETTE.length],
        }));

        const gLinks: GraphLink[] = data.edges.map((e) => ({
          id: e.id,
          source: e.source_concept_id,
          target: e.target_concept_id,
          relation: e.relation,
          created_via: e.created_via,
        }));

        setNodes(gNodes);
        setLinks(gLinks);
      })
      .catch((err) => console.error("Failed to fetch graph", err))
      .finally(() => setLoading(false));
  }, []);

  const selectedNode = nodes.find((n) => n.id === selId);
  const selectedLinks = links.filter(
    (l) => {
      const src = typeof l.source === 'object' ? (l.source as any).id : l.source;
      const tgt = typeof l.target === 'object' ? (l.target as any).id : l.target;
      return src === selId || tgt === selId;
    }
  );

  const handleNodeClick = useCallback((node: any) => {
    setSelId(node.id);
    if (fgRef.current) {
      fgRef.current.centerAt(node.x, node.y, 400);
      fgRef.current.zoom(3, 400);
    }
  }, []);

  const nodeCanvasObject = useCallback((node: any, ctx: CanvasRenderingContext2D, globalScale: number) => {
    const isSel = node.id === selId;
    const r = Math.sqrt(node.val || 1) * 4;
    const fontSize = Math.max(10 / globalScale, 3);

    // Node circle
    ctx.beginPath();
    ctx.arc(node.x, node.y, r, 0, 2 * Math.PI);
    ctx.fillStyle = isSel ? node.color : `${node.color}88`;
    ctx.fill();
    ctx.strokeStyle = isSel ? "#fff" : `${node.color}`;
    ctx.lineWidth = isSel ? 2 / globalScale : 1 / globalScale;
    ctx.stroke();

    // Label
    if (globalScale > 1.2 || isSel) {
      ctx.font = `${isSel ? 'bold ' : ''}${fontSize}px Inter, sans-serif`;
      ctx.textAlign = "center";
      ctx.textBaseline = "top";
      ctx.fillStyle = isSel ? node.color : "#6b655e";
      ctx.fillText(node.name, node.x, node.y + r + 2);
    }
  }, [selId]);

  const linkCanvasObject = useCallback((link: any, ctx: CanvasRenderingContext2D, globalScale: number) => {
    const src = link.source;
    const tgt = link.target;
    if (!src || !tgt || typeof src.x !== 'number') return;

    const srcId = typeof src === 'object' ? src.id : src;
    const tgtId = typeof tgt === 'object' ? tgt.id : tgt;
    const isHighlighted = srcId === selId || tgtId === selId;
    
    ctx.beginPath();
    ctx.moveTo(src.x, src.y);
    ctx.lineTo(tgt.x, tgt.y);
    ctx.strokeStyle = isHighlighted ? (REL_COLORS[link.relation] || "#c96442") : "#e4e0d833";
    ctx.lineWidth = isHighlighted ? 1.5 / globalScale : 0.5 / globalScale;
    ctx.stroke();

    // Show relation label when highlighted
    if (isHighlighted && globalScale > 1.5) {
      const midX = (src.x + tgt.x) / 2;
      const midY = (src.y + tgt.y) / 2;
      const fontSize = Math.max(8 / globalScale, 3);
      ctx.font = `${fontSize}px 'JetBrains Mono', monospace`;
      ctx.textAlign = "center";
      ctx.fillStyle = REL_COLORS[link.relation] || "#9a938a";
      ctx.fillText(link.relation.replace(/_/g, " "), midX, midY - 3 / globalScale);
    }
  }, [selId]);

  return (
    <div className="graph" style={{ display: "flex", width: "100%", height: "100%" }}>
      {/* Canvas */}
      <div style={{ flex: 1, position: "relative", background: "var(--bg)" }}>
        {loading && (
          <div style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center", zIndex: 5 }}>
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--ink-4)" }}>Loading graph...</span>
          </div>
        )}

        {!loading && nodes.length === 0 && (
          <div style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center", zIndex: 5 }}>
            <div style={{ textAlign: "center", color: "var(--ink-4)" }}>
              <div style={{ fontSize: 32, marginBottom: 8 }}>🕸️</div>
              <div style={{ fontSize: 14, fontWeight: 500 }}>Your knowledge graph is empty</div>
              <div style={{ fontSize: 12, marginTop: 4 }}>Upload and process documents to build your concept network.</div>
            </div>
          </div>
        )}

        {!loading && nodes.length > 0 && (
          <ForceGraph2D
            ref={fgRef}
            graphData={{ nodes, links }}
            nodeCanvasObject={nodeCanvasObject}
            linkCanvasObject={linkCanvasObject}
            onNodeClick={handleNodeClick}
            onBackgroundClick={() => setSelId(null)}
            nodeLabel=""
            cooldownTicks={80}
            d3AlphaDecay={0.02}
            d3VelocityDecay={0.3}
          />
        )}

        {/* Legend */}
        <div style={{
          position: "absolute", bottom: 16, left: 16, display: "flex", gap: 12,
          background: "var(--bg)", border: "1px solid var(--rule)", borderRadius: 4,
          padding: "6px 12px", fontSize: 10, fontFamily: "var(--font-mono)",
        }}>
          {Object.entries(REL_COLORS).map(([rel, color]) => (
            <div key={rel} style={{ display: "flex", alignItems: "center", gap: 4 }}>
              <span style={{ width: 8, height: 8, borderRadius: "50%", background: color, display: "inline-block" }} />
              {rel.replace(/_/g, " ")}
            </div>
          ))}
        </div>
      </div>

      {/* Detail Pane */}
      <aside style={{
        width: 300, borderLeft: "1px solid var(--rule)", background: "var(--bg)",
        display: "flex", flexDirection: "column", padding: 24, overflow: "auto",
      }}>
        {selectedNode ? (
          <>
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)", textTransform: "uppercase" as const, letterSpacing: "0.08em", marginBottom: 8 }}>
              Concept
            </div>
            <h2 style={{ fontSize: 18, margin: "0 0 10px", fontWeight: 600, letterSpacing: "-0.01em" }}>
              {selectedNode.name}
            </h2>
            <p style={{ fontSize: 13, color: "var(--ink-3)", lineHeight: 1.55, marginBottom: 20 }}>
              {selectedNode.description || "No description generated yet."}
            </p>

            {/* Stats */}
            <div style={{
              display: "grid", gridTemplateColumns: "90px 1fr", gap: "6px 0",
              fontSize: 12, borderTop: "1px solid var(--rule)", paddingTop: 14, marginBottom: 18,
            }}>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)" }}>Connections</span>
              <span>{selectedLinks.length}</span>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)" }}>Created</span>
              <span>{new Date(selectedNode.created_at).toLocaleDateString()}</span>
            </div>

            {/* Connections */}
            <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)", textTransform: "uppercase" as const, letterSpacing: "0.06em", marginBottom: 8 }}>
              Relations
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {selectedLinks.map((l) => {
                const srcId = typeof l.source === 'object' ? (l.source as any).id : l.source;
                const tgtId = typeof l.target === 'object' ? (l.target as any).id : l.target;
                const peerId = srcId === selId ? tgtId : srcId;
                const peer = nodes.find((n) => n.id === peerId);
                const isOutgoing = srcId === selId;

                return (
                  <div
                    key={l.id}
                    style={{
                      display: "flex", alignItems: "center", gap: 8,
                      padding: "6px 8px", borderRadius: 4, cursor: "pointer",
                      border: "1px solid var(--rule)",
                    }}
                    onClick={() => { setSelId(peerId); if (fgRef.current && peer) { fgRef.current.centerAt((peer as any).x, (peer as any).y, 400); }}}
                  >
                    <span style={{
                      fontSize: 9, fontFamily: "var(--font-mono)", fontWeight: 600,
                      color: REL_COLORS[l.relation] || "var(--ink-4)",
                      border: `1px solid ${REL_COLORS[l.relation] || "var(--rule)"}`,
                      borderRadius: 10, padding: "1px 6px",
                    }}>
                      {l.relation.replace(/_/g, " ")}
                    </span>
                    <span style={{ fontSize: 12, flex: 1 }}>
                      {isOutgoing ? "→ " : "← "}{peer?.name || "Unknown"}
                    </span>
                  </div>
                );
              })}
              {selectedLinks.length === 0 && (
                <div style={{ fontSize: 12, color: "var(--ink-4)", fontStyle: "italic" }}>No relations yet.</div>
              )}
            </div>
          </>
        ) : (
          <div style={{ margin: "auto", textAlign: "center", color: "var(--ink-4)" }}>
            <div style={{ fontSize: 24, marginBottom: 8 }}>🔍</div>
            <div style={{ fontSize: 13 }}>Click a node to inspect</div>
            <div style={{ fontSize: 11, marginTop: 4, color: "var(--ink-4)" }}>Scroll to zoom · Drag to pan</div>
          </div>
        )}
      </aside>
    </div>
  );
}
