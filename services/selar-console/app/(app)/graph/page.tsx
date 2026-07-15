// page.tsx — Redesigned Knowledge Graph view with force-directed layout and high-fidelity controls.
// Uses react-force-graph-2d with enhanced node rendering, particle flow, and glassmorphic overlays.

"use client";

import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import { Icon } from "@/components/ui/Icon";
import { clientFetch, type GraphData, type GraphNodeType } from "@/lib/api";
import dynamic from "next/dynamic";
import type { ForceGraphMethods, LinkObject, NodeObject } from "react-force-graph-2d";

type ForceGraphComponent = (typeof import("react-force-graph-2d"))["default"];
const ForceGraph2D = dynamic(() => import("react-force-graph-2d"), { ssr: false }) as ForceGraphComponent;

interface GraphNode {
  id: string;
  name: string;
  description: string;
  created_at: string;
  nodeType: GraphNodeType;
  state: string;
  documentTitle?: string;
  val: number; // size
  color: string;
  x?: number;
  y?: number;
}
interface GraphLinkMetadata {
  id: string;
  relation: string;
  created_via: string;
  state: string;
  confidence?: number;
  explanation?: string;
}

type RenderNode = NodeObject<GraphNode>;
type GraphLink = LinkObject<GraphNode, GraphLinkMetadata>;

function endpointId(endpoint: GraphLink["source"]): string | undefined {
  const value = typeof endpoint === "object" ? endpoint.id : endpoint;
  return value === undefined ? undefined : String(value);
}

const REL_COLORS: Record<string, string> = {
  prerequisite_of: "#c96442",
  related_to: "#9a938a",
  sub_concept_of: "#8a6a3d",
  contradicts: "#c0443a",
  extends: "#7a8c5c",
  concept_overlap: "#5e8aaa",
  claim_extension: "#7a8c5c",
  assumption_conflict: "#c0443a",
  question_resolution: "#9a6a9a",
  has_claim: "#c8c1b7",
  has_assumption: "#c8c1b7",
  raises: "#c8c1b7",
};

const NODE_TYPE_COLORS: Record<GraphNodeType, string> = {
  concept: "#5e8aaa",
  document: "#c96442",
  claim: "#7a8c5c",
  assumption: "#8a6a3d",
  question: "#9a6a9a",
};

export default function GraphPage() {
  const [nodes, setNodes] = useState<GraphNode[]>([]);
  const [links, setLinks] = useState<GraphLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [selId, setSelId] = useState<string | null>(null);
  const [hoverNode, setHoverNode] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [isPhysicsPaused, setIsPhysicsPaused] = useState(false);
  const fgRef = useRef<ForceGraphMethods<GraphNode, GraphLinkMetadata> | undefined>(undefined);
  const containerRef = useRef<HTMLDivElement>(null);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });

  // Measure container dimensions dynamically to prevent canvas overflow
  useEffect(() => {
    if (!containerRef.current) return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        setDimensions({ width, height });
      }
    });
    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    clientFetch<GraphData>("/api/graph")
      .then((data) => {
        const connCount: Record<string, number> = {};
        for (const e of data.edges) {
          connCount[e.source] = (connCount[e.source] || 0) + 1;
          connCount[e.target] = (connCount[e.target] || 0) + 1;
        }

        const gNodes: GraphNode[] = data.nodes.map((n) => ({
          id: n.id,
          name: n.name,
          description: n.description,
          created_at: n.created_at,
          nodeType: n.node_type,
          state: n.state,
          documentTitle: n.document_title,
          val: Math.max(3, (connCount[n.id] || 0) + 2),
          color: NODE_TYPE_COLORS[n.node_type] || "#9a938a",
        }));

        const gLinks: GraphLink[] = data.edges.map((e) => ({
          id: e.id,
          source: e.source,
          target: e.target,
          relation: e.relation,
          created_via: e.created_via,
          state: e.state,
          confidence: e.confidence,
          explanation: e.explanation,
        }));

        setNodes(gNodes);
        setLinks(gLinks);
      })
      .catch((err) => console.error("Failed to fetch graph", err))
      .finally(() => setLoading(false));
  }, []);

  // Adjust simulation forces once nodes are loaded
  useEffect(() => {
    if (!loading && nodes.length > 0) {
      let fitTimer: ReturnType<typeof setTimeout> | undefined;
      const timer = setTimeout(() => {
        if (fgRef.current) {
          // De-clutter: repel nodes strongly and increase link distance
          fgRef.current.d3Force("charge")?.strength(-800);
          fgRef.current.d3Force("link")?.distance(180);
          fgRef.current.d3ReheatSimulation();
          fitTimer = setTimeout(() => fgRef.current?.zoomToFit(500, 80), 1200);
        }
      }, 400); // Delay to ensure canvas has mounted and D3 engine is ready
      return () => {
        clearTimeout(timer);
        if (fitTimer) clearTimeout(fitTimer);
      };
    }
  }, [nodes, loading]);

  const selectedNode = useMemo(() => nodes.find((n) => n.id === selId), [nodes, selId]);

  const selectedLinks = useMemo(() => links.filter(
    (l) => {
      const src = endpointId(l.source);
      const tgt = endpointId(l.target);
      return src === selId || tgt === selId;
    }
  ), [links, selId]);

  // Compute connected nodes for highlighting
  const connectedNodes = useMemo(() => {
    const set = new Set<string>();
    if (!hoverNode) return set;
    set.add(hoverNode);
    for (const l of links) {
      const srcId = endpointId(l.source);
      const tgtId = endpointId(l.target);
      if (srcId === hoverNode && tgtId) set.add(tgtId);
      if (tgtId === hoverNode && srcId) set.add(srcId);
    }
    return set;
  }, [links, hoverNode]);

  const handleNodeClick = useCallback((node: RenderNode) => {
    setSelId(node.id);
    if (fgRef.current) {
      fgRef.current.centerAt(node.x, node.y, 600);
      fgRef.current.zoom(2.5, 600);
    }
  }, []);

  const zoomIn = () => {
    if (fgRef.current) {
      const currentZoom = fgRef.current.zoom();
      fgRef.current.zoom(Math.min(currentZoom + 0.5, 6), 300);
    }
  };

  const zoomOut = () => {
    if (fgRef.current) {
      const currentZoom = fgRef.current.zoom();
      fgRef.current.zoom(Math.max(currentZoom - 0.5, 0.5), 300);
    }
  };

  const recenter = () => {
    if (fgRef.current) {
      fgRef.current.zoomToFit(600, 60);
      setSelId(null);
    }
  };

  const togglePhysics = () => {
    if (fgRef.current) {
      if (isPhysicsPaused) {
        fgRef.current.resumeAnimation();
      } else {
        fgRef.current.pauseAnimation();
      }
      setIsPhysicsPaused(!isPhysicsPaused);
    }
  };

  // Node drawing with high quality shadows and glowing styles
  const nodeCanvasObject = useCallback((node: RenderNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
    if (typeof node.x !== 'number' || typeof node.y !== 'number' || !isFinite(node.x) || !isFinite(node.y)) {
      return;
    }
    const isSel = node.id === selId;
    const isHovered = node.id === hoverNode;
    const isDimmed = hoverNode !== null && !connectedNodes.has(node.id);

    const r = Math.sqrt(node.val || 1) * 4.2;
    const fontSize = Math.max(10 / globalScale, 4.5);

    ctx.save();

    // Dimmed effect for highlighting path context
    ctx.globalAlpha = isDimmed ? 0.20 : 1.0;

    // Outer Glow / Ring
    if (isSel || isHovered) {
      ctx.beginPath();
      ctx.arc(node.x, node.y, r + (isSel ? 4.5 : 2.5), 0, 2 * Math.PI);
      ctx.fillStyle = isSel ? `${node.color}1c` : `${node.color}10`;
      ctx.fill();
      ctx.strokeStyle = isSel ? node.color : `${node.color}66`;
      ctx.lineWidth = 1.2 / globalScale;
      ctx.stroke();
    }

    // Modern drop shadow for the node sphere
    ctx.shadowColor = "rgba(26, 24, 22, 0.16)";
    ctx.shadowBlur = 6 * globalScale;
    ctx.shadowOffsetX = 0;
    ctx.shadowOffsetY = 2.5 * globalScale;

    // Main Node Circle (Flat & Clean color fill)
    ctx.beginPath();
    ctx.arc(node.x, node.y, r, 0, 2 * Math.PI);
    ctx.fillStyle = node.color;
    ctx.fill();

    // Reset shadow for borders and text
    ctx.shadowColor = "transparent";

    // Node Border
    ctx.strokeStyle = isSel ? "#ffffff" : "rgba(255, 255, 255, 0.65)";
    ctx.lineWidth = isSel ? 2.5 / globalScale : 1.2 / globalScale;
    ctx.stroke();

    // Node Label (Always show selected or zoomed nodes)
    const showLabel = globalScale > 1.1 || node.nodeType === "document"
      || node.nodeType === "concept" || isSel || isHovered;
    if (showLabel) {
      ctx.font = `${isSel ? '600' : '500'} ${fontSize}px sans-serif`;
      ctx.textAlign = "center";
      ctx.textBaseline = "top";

      // Draw outline for clear contrast instead of overlapping solid background blocks
      ctx.strokeStyle = "#faf9f7";
      ctx.lineWidth = 4 / globalScale;
      ctx.strokeText(node.name, node.x, node.y + r + 5);

      ctx.fillStyle = isSel ? "#1a1816" : "#46413c";
      ctx.fillText(node.name, node.x, node.y + r + 5);
    }

    ctx.restore();
  }, [selId, hoverNode, connectedNodes]);


  const linkCanvasObject = useCallback((link: GraphLink, ctx: CanvasRenderingContext2D, globalScale: number) => {
    const src = link.source;
    const tgt = link.target;
    if (typeof src !== "object" || typeof tgt !== "object"
      || typeof src.x !== "number" || typeof src.y !== "number"
      || typeof tgt.x !== "number" || typeof tgt.y !== "number") return;

    const srcId = src.id === undefined ? undefined : String(src.id);
    const tgtId = tgt.id === undefined ? undefined : String(tgt.id);

    const isSelectedPath = srcId === selId || tgtId === selId;
    const isHoveredPath = hoverNode !== null && (srcId === hoverNode || tgtId === hoverNode);
    const isDimmed = hoverNode !== null && !isHoveredPath;

    ctx.save();

    ctx.beginPath();
    ctx.moveTo(src.x, src.y);
    ctx.lineTo(tgt.x, tgt.y);

    if (isDimmed) {
      ctx.strokeStyle = "#ded9d2";
      ctx.globalAlpha = 0.08;
      ctx.lineWidth = 0.5 / globalScale;
    } else if (isSelectedPath || isHoveredPath) {
      ctx.strokeStyle = REL_COLORS[link.relation] || "#c96442";
      ctx.globalAlpha = 0.95;
      ctx.lineWidth = 2.0 / globalScale;
    } else {
      ctx.strokeStyle = "#c8c1b7";
      ctx.globalAlpha = 0.45;
      ctx.lineWidth = 0.8 / globalScale;
    }

    ctx.stroke();

    // Show relation label when hovered/selected path
    if ((isSelectedPath || isHoveredPath) && globalScale > 1.4) {
      const midX = (src.x + tgt.x) / 2;
      const midY = (src.y + tgt.y) / 2;
      const fontSize = Math.max(9 / globalScale, 4);
      ctx.font = `${fontSize}px monospace`;
      ctx.textAlign = "center";
      ctx.textBaseline = "middle";

      const labelText = link.relation.replace(/_/g, " ");
      const labelWidth = ctx.measureText(labelText).width;

      // Label text badge background
      ctx.fillStyle = "rgba(250, 249, 247, 0.9)";
      ctx.fillRect(midX - labelWidth / 2 - 2, midY - fontSize / 2 - 1, labelWidth + 4, fontSize + 2);

      ctx.fillStyle = REL_COLORS[link.relation] || "#c96442";
      ctx.fillText(labelText, midX, midY);
    }

    ctx.restore();
  }, [selId, hoverNode]);

  // Filter nodes matching search query
  const filteredNodes = useMemo(() => {
    if (!searchQuery.trim()) return [];
    return nodes.filter(n => n.name.toLowerCase().includes(searchQuery.toLowerCase()));
  }, [nodes, searchQuery]);

  const handleSearchSelect = (node: GraphNode) => {
    setSearchQuery("");
    handleNodeClick(node);
  };

  return (
    <div className="graph" style={{ display: "flex", width: "100%", height: "100%", position: "relative", overflow: "hidden" }}>

      {/* Search and Control Deck (Floating overlay top-left) */}
      <div style={{
        position: "absolute", top: 16, left: 16, zIndex: 10,
        display: "flex", gap: 10, alignItems: "center"
      }}>
        {/* Concept Finder Search Bar */}
        <div style={{ position: "relative" }}>
          <div style={{
            display: "flex", alignItems: "center", gap: 8,
            background: "rgba(250, 249, 247, 0.8)", backdropFilter: "blur(12px)",
            border: "1px solid var(--rule)", borderRadius: "var(--r-md)",
            padding: "6px 12px", width: 240, boxShadow: "var(--shadow-1)",
            transition: "all 0.2s ease"
          }}>
            <Icon name="search" size={13} style={{ color: "var(--ink-3)" }} />
            <input
              type="text"
              placeholder="Search the knowledge graph..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              style={{
                background: "none", border: "none", outline: "none",
                fontSize: 12, color: "var(--ink)", width: "100%",
                fontFamily: "var(--font-sans)"
              }}
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery("")}
                style={{ background: "none", border: "none", cursor: "pointer", color: "var(--ink-4)" }}
              >
                <Icon name="x" size={10} />
              </button>
            )}
          </div>

          {/* Autocomplete Dropdown */}
          {filteredNodes.length > 0 && (
            <div style={{
              position: "absolute", top: "calc(100% + 6px)", left: 0, right: 0,
              background: "rgba(250, 249, 247, 0.95)", backdropFilter: "blur(16px)",
              border: "1px solid var(--rule)", borderRadius: "var(--r-md)",
              boxShadow: "var(--shadow-2)", overflow: "hidden", zIndex: 20,
              maxHeight: 200, overflowY: "auto"
            }}>
              {filteredNodes.map((n) => (
                <div
                  key={n.id}
                  onClick={() => handleSearchSelect(n)}
                  style={{
                    padding: "8px 12px", fontSize: 12, color: "var(--ink-2)",
                    cursor: "pointer", borderBottom: "1px solid var(--rule)",
                    display: "flex", alignItems: "center", gap: 8,
                    transition: "background 0.15s ease"
                  }}
                  className="hover-bg-2"
                >
                  <span style={{ width: 8, height: 8, borderRadius: "50%", background: n.color }} />
                  <span style={{ fontWeight: 500 }}>{n.name}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Toolbar Controls */}
        <div style={{
          display: "flex", gap: 2,
          background: "rgba(250, 249, 247, 0.8)", backdropFilter: "blur(12px)",
          border: "1px solid var(--rule)", borderRadius: "var(--r-md)",
          padding: 2, boxShadow: "var(--shadow-1)"
        }}>
          <button onClick={zoomIn} title="Zoom In" style={{
            background: "none", border: "none", padding: "6px 8px", cursor: "pointer",
            borderRadius: "var(--r-sm)", color: "var(--ink-2)", display: "flex"
          }} className="hover-bg-2">
            <Icon name="zoom_in" size={13} />
          </button>
          <button onClick={zoomOut} title="Zoom Out" style={{
            background: "none", border: "none", padding: "6px 8px", cursor: "pointer",
            borderRadius: "var(--r-sm)", color: "var(--ink-2)", display: "flex"
          }} className="hover-bg-2">
            <Icon name="zoom_out" size={13} />
          </button>
          <button onClick={recenter} title="Fit Network" style={{
            background: "none", border: "none", padding: "6px 8px", cursor: "pointer",
            borderRadius: "var(--r-sm)", color: "var(--ink-2)", display: "flex"
          }} className="hover-bg-2">
            <Icon name="eye" size={13} />
          </button>
          <div style={{ width: 1, background: "var(--rule)", margin: "4px 2px" }} />
          <button onClick={togglePhysics} title={isPhysicsPaused ? "Resume Forces" : "Pause Forces"} style={{
            background: "none", border: "none", padding: "6px 8px", cursor: "pointer",
            borderRadius: "var(--r-sm)", color: isPhysicsPaused ? "var(--accent)" : "var(--ink-2)", display: "flex"
          }} className="hover-bg-2">
            <Icon name="bolt" size={13} />
          </button>
        </div>
      </div>

      {/* Main Graph Viewport */}
      <div ref={containerRef} style={{
        flex: 1, position: "relative",
        background: "var(--bg)",
        // Premium subtle blueprint grid pattern
        backgroundImage: "radial-gradient(var(--rule-2) 1px, transparent 1px)",
        backgroundSize: "20px 20px"
      }}>
        {loading && (
          <div style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center", zIndex: 5, backdropFilter: "blur(4px)" }}>
            <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 12 }}>
              <Icon name="spinner" size={24} className="animate-spin" style={{ color: "var(--accent)" }} />
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--ink-3)" }}>Assembling neural network...</span>
            </div>
          </div>
        )}

        {!loading && nodes.length === 0 && (
          <div style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center", zIndex: 5 }}>
            <div style={{ textAlign: "center", maxWidth: 280, padding: 24 }}>
              <div style={{ fontSize: 32, marginBottom: 12 }}>🕸️</div>
              <div style={{ fontSize: 15, fontWeight: 600, color: "var(--ink)" }}>No mental models mapped yet</div>
              <div style={{ fontSize: 12, marginTop: 6, color: "var(--ink-3)", lineHeight: 1.5 }}>
                Process documents to create evidence-backed claims, concepts, assumptions, and open questions.
              </div>
            </div>
          </div>
        )}

        {!loading && nodes.length > 0 && (
          <ForceGraph2D<GraphNode, GraphLinkMetadata>
            ref={fgRef}
            width={dimensions.width}
            height={dimensions.height}
            graphData={{ nodes, links }}
            nodeCanvasObject={nodeCanvasObject}
            linkCanvasObject={linkCanvasObject}
            onNodeClick={handleNodeClick}
            onNodeHover={(node) => setHoverNode(node?.id === undefined ? null : String(node.id))}
            onBackgroundClick={() => setSelId(null)}
            nodeLabel=""
            cooldownTicks={80}
            d3AlphaDecay={0.02}
            d3VelocityDecay={0.35}
            // Particle animations along links to represent direction and logic flow
            linkDirectionalParticles={(link) => {
              const srcId = endpointId(link.source);
              const tgtId = endpointId(link.target);
              const isDirectPath = srcId === selId || tgtId === selId || srcId === hoverNode || tgtId === hoverNode;
              return isDirectPath ? 3 : 0.8;
            }}
            linkDirectionalParticleSpeed={0.006}
            linkDirectionalParticleWidth={(link) => {
              const srcId = endpointId(link.source);
              const tgtId = endpointId(link.target);
              return (srcId === selId || tgtId === selId) ? 2.2 : 1.2;
            }}
            linkDirectionalParticleColor={(link) => REL_COLORS[link.relation] || "var(--accent)"}
          />
        )}

        {/* Legend Panel (Floating bottom-left) */}
        <div style={{
          position: "absolute", bottom: 16, left: 16, display: "flex", flexDirection: "column", gap: 6,
          background: "rgba(250, 249, 247, 0.8)", backdropFilter: "blur(12px)",
          border: "1px solid var(--rule)", borderRadius: "var(--r-md)",
          padding: "8px 12px", fontSize: 10, fontFamily: "var(--font-mono)",
          boxShadow: "var(--shadow-1)"
        }}>
          <div style={{ color: "var(--ink-4)", textTransform: "uppercase", letterSpacing: "0.05em", fontWeight: 600, marginBottom: 4 }}>
            Relationship Types
          </div>
          <div style={{ display: "flex", gap: 10, flexWrap: "wrap", maxWidth: 400 }}>
            {Object.entries(REL_COLORS).map(([rel, color]) => (
              <div key={rel} style={{ display: "flex", alignItems: "center", gap: 6, color: "var(--ink-2)" }}>
                <span style={{ width: 8, height: 8, borderRadius: "50%", background: color, display: "inline-block" }} />
                <span>{rel.replace(/_/g, " ")}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Inspect / Detail Side Panel */}
      <aside style={{
        width: 340, borderLeft: "1px solid var(--rule)", background: "rgba(250, 249, 247, 0.5)",
        backdropFilter: "blur(20px)", display: "flex", flexDirection: "column", padding: "24px 20px",
        overflowY: "auto", zIndex: 10, boxShadow: "-2px 0 20px rgba(0,0,0,0.02)"
      }}>
        {selectedNode ? (
          <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>

            {/* Header Badge */}
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12 }}>
              <span style={{
                fontSize: 9, fontFamily: "var(--font-mono)", fontWeight: 600,
                color: selectedNode.color, background: `${selectedNode.color}15`,
                border: `1px solid ${selectedNode.color}33`,
                borderRadius: 4, padding: "2px 8px", textTransform: "uppercase"
              }}>
                {selectedNode.nodeType.replace(/_/g, " ")} · {selectedNode.state}
              </span>
              <span style={{ flex: 1 }} />
              <button
                onClick={() => setSelId(null)}
                style={{
                  background: "none", border: "none", cursor: "pointer",
                  color: "var(--ink-4)", display: "flex", padding: 4, borderRadius: "50%"
                }}
                className="hover-bg-2"
              >
                <Icon name="x" size={12} />
              </button>
            </div>

            {/* Title */}
            <h2 style={{
              fontSize: 22, margin: "0 0 14px 0", fontWeight: 700,
              color: "var(--ink)", fontFamily: "var(--font-serif)",
              letterSpacing: "-0.01em", lineHeight: 1.2
            }}>
              {selectedNode.name}
            </h2>

            {/* Description Card */}
            <div style={{
              background: "var(--bg)", border: "1px solid var(--rule)",
              borderRadius: "var(--r-md)", padding: 14, marginBottom: 20,
              boxShadow: "var(--shadow-1)"
            }}>
              <p style={{ fontSize: 13, color: "var(--ink-2)", lineHeight: 1.6, margin: 0 }}>
                {selectedNode.description || "No description has been generated for this node."}
              </p>
            </div>

            {/* Stats Block */}
            <div style={{
              display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10,
              borderTop: "1px solid var(--rule)", borderBottom: "1px solid var(--rule)",
              padding: "12px 0", marginBottom: 20
            }}>
              <div>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 9, color: "var(--ink-4)", display: "block", textTransform: "uppercase" }}>
                  Degree Centrality
                </span>
                <span style={{ fontSize: 16, fontWeight: 600, color: "var(--ink-2)" }}>
                  {selectedLinks.length} connection{selectedLinks.length !== 1 ? 's' : ''}
                </span>
              </div>
              <div>
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 9, color: "var(--ink-4)", display: "block", textTransform: "uppercase" }}>
                  Discovered On
                </span>
                <span style={{ fontSize: 14, fontWeight: 500, color: "var(--ink-2)", display: "block", marginTop: 2 }}>
                  {new Date(selectedNode.created_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}
                </span>
              </div>
            </div>

            {/* Related Concept Links */}
            <div style={{
              fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)",
              textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 10,
              fontWeight: 600
            }}>
              Structural Paths ({selectedLinks.length})
            </div>

            <div style={{ display: "flex", flexDirection: "column", gap: 8, overflowY: "auto", flex: 1, paddingRight: 4 }}>
              {selectedLinks.map((l) => {
                const srcId = endpointId(l.source);
                const tgtId = endpointId(l.target);
                const peerId = srcId === selId ? tgtId : srcId;
                const peer = nodes.find((n) => n.id === peerId);
                const isOutgoing = srcId === selId;
                const relColor = REL_COLORS[l.relation] || "var(--accent)";

                return (
                  <div
                    key={l.id}
                    onClick={() => {
                      if (peerId) setSelId(peerId);
                      if (fgRef.current && peer) {
                        fgRef.current.centerAt(peer.x, peer.y, 600);
                      }
                    }}
                    style={{
                      display: "flex", flexDirection: "column", gap: 6,
                      padding: "10px 12px", borderRadius: "var(--r-md)", cursor: "pointer",
                      border: "1px solid var(--rule)", background: "var(--bg)",
                      transition: "all 0.2s cubic-bezier(0.16, 1, 0.3, 1)",
                      boxShadow: "var(--shadow-1)"
                    }}
                    className="hover-card"
                  >
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <span style={{
                        fontSize: 8, fontFamily: "var(--font-mono)", fontWeight: 700,
                        color: relColor, background: `${relColor}15`,
                        border: `1px solid ${relColor}33`,
                        borderRadius: 10, padding: "1px 6px", textTransform: "uppercase"
                      }}>
                        {l.relation.replace(/_/g, " ")}
                      </span>
                      <span style={{ flex: 1 }} />
                      <span style={{ fontSize: 11, color: "var(--ink-4)", fontFamily: "var(--font-mono)" }}>
                        {isOutgoing ? "outgoing →" : "← incoming"}
                      </span>
                    </div>
                    <span style={{ fontSize: 13, fontWeight: 600, color: "var(--ink-2)" }}>
                      {peer?.name || "Loading peer..."}
                    </span>
                  </div>
                );
              })}
              {selectedLinks.length === 0 && (
                <div style={{ display: "grid", placeItems: "center", height: 80, border: "1px dashed var(--rule)", borderRadius: "var(--r-md)" }}>
                  <span style={{ fontSize: 12, color: "var(--ink-4)", fontStyle: "italic" }}>No concept connections yet.</span>
                </div>
              )}
            </div>
          </div>
        ) : (
          <div style={{ margin: "auto", textAlign: "center", maxWidth: 220 }}>
            <div style={{
              width: 48, height: 48, borderRadius: "50%", background: "var(--bg)",
              border: "1px solid var(--rule)", display: "grid", placeItems: "center",
              margin: "0 auto 16px auto", boxShadow: "var(--shadow-1)", color: "var(--ink-3)"
            }}>
              <Icon name="graph" size={18} />
            </div>
            <div style={{ fontSize: 14, fontWeight: 600, color: "var(--ink-2)", marginBottom: 4 }}>
              Knowledge Topology
            </div>
            <div style={{ fontSize: 11, color: "var(--ink-4)", lineHeight: 1.5 }}>
              Click any node in the interactive network to inspect its semantic bounds, cross-references, and definitions.
            </div>
          </div>
        )}
      </aside>
    </div>
  );
}
