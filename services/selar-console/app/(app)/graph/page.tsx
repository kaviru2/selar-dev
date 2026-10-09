// page.tsx — Redesigned Knowledge Graph view with force-directed layout and high-fidelity controls.
// Uses react-force-graph-2d with enhanced node rendering, particle flow, and glassmorphic overlays.

"use client";

import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import { Icon } from "@/components/ui/Icon";
import { EmptyState } from "@/components/ui/EmptyState";
import { clientFetch, type GraphData, type GraphNodeType, type ReplayReport } from "@/lib/api";
import dynamic from "next/dynamic";
import Link from "next/link";
import { AssertionProvenance } from "@/components/AssertionProvenance";
import type { ForceGraphMethods, LinkObject, NodeObject } from "react-force-graph-2d";
import styles from "./graph.module.css";
import { NODE_TYPE_COLORS, REL_COLORS, useGraphColors } from "@/lib/graph-theme";
import { candidateReviewURL, crossReadingNotice, crossReadingSummary, isCandidateLink, linkDash, linkLayer } from "@/lib/graph-links";

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
  valid_from?: string;
  valid_to?: string;
  observed_at?: string;
  superseded_by?: string;
  mental_link_id?: string;
  source_document_id?: string;
  target_document_id?: string;
  source_quote?: string;
  target_quote?: string;
  review_revision?: number;
  assertion_id?: string;
  assertion_scope?: "own_work" | "reported_about_other";
  asserting_document_title?: string;
  candidate_link_id?: string;
}

type RenderNode = NodeObject<GraphNode>;
type GraphLink = LinkObject<GraphNode, GraphLinkMetadata>;

function endpointId(endpoint: GraphLink["source"]): string | undefined {
  const value = typeof endpoint === "object" ? endpoint.id : endpoint;
  return value === undefined ? undefined : String(value);
}

function isInteractionLink(link: GraphLinkMetadata): boolean {
  return linkLayer(link) === "yours";
}

export default function GraphPage() {
  const canvasColor = useGraphColors();
  const [nodes, setNodes] = useState<GraphNode[]>([]);
  const [links, setLinks] = useState<GraphLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [selId, setSelId] = useState<string | null>(null);
  const [hoverNode, setHoverNode] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [isPhysicsPaused, setIsPhysicsPaused] = useState(false);
  const [reconciling, setReconciling] = useState(false);
  const [showPdfKnowledge, setShowPdfKnowledge] = useState(true);
  const [showInteractionChanges, setShowInteractionChanges] = useState(true);
  const [showCandidates, setShowCandidates] = useState(true);
  const [noticeDismissed, setNoticeDismissed] = useState(false);
  const fgRef = useRef<ForceGraphMethods<GraphNode, GraphLinkMetadata> | undefined>(undefined);
  const containerRef = useRef<HTMLDivElement>(null);
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });
  const visibleLinks = useMemo(() => links.filter((link) => {
    const layer = linkLayer(link);
    return layer === "candidate" ? showCandidates : layer === "yours" ? showInteractionChanges : showPdfKnowledge;
  }), [links, showCandidates, showInteractionChanges, showPdfKnowledge]);

  const visibleNodes = useMemo(() => {
    if (showPdfKnowledge) return nodes;
    if (!showInteractionChanges && !showCandidates) return [];
    const connected = new Set<string>();
    for (const link of visibleLinks) {
      const source = endpointId(link.source);
      const target = endpointId(link.target);
      if (source) connected.add(source);
      if (target) connected.add(target);
    }
    return nodes.filter((node) => connected.has(node.id));
  }, [nodes, showCandidates, showInteractionChanges, showPdfKnowledge, visibleLinks]);

  const graphData = useMemo(() => ({ nodes: visibleNodes, links: visibleLinks }), [visibleNodes, visibleLinks]);
  const provenanceCounts = useMemo(() => ({
    pdf: links.filter((link) => linkLayer(link) === "pdf").length,
    interaction: links.filter((link) => linkLayer(link) === "yours").length,
    candidate: links.filter((link) => linkLayer(link) === "candidate").length,
  }), [links]);
  const readingNotice = useMemo(() => crossReadingNotice(crossReadingSummary(
    nodes.map((n) => ({ id: n.id, node_type: n.nodeType })),
    links.map((l) => ({ source: endpointId(l.source) ?? "", target: endpointId(l.target) ?? "", state: l.state, created_via: l.created_via, candidate_link_id: l.candidate_link_id })),
  ), showCandidates), [nodes, links, showCandidates]);
  const interactionNodeImpact = useMemo(() => {
    const impact = new Map<string, "active" | "rolled-back">();
    for (const link of links.filter(isInteractionLink)) {
      const linkImpact = ["rejected", "archived", "superseded"].includes(link.state) ? "rolled-back" : "active";
      for (const id of [endpointId(link.source), endpointId(link.target)]) {
        if (!id) continue;
        if (linkImpact === "active" || !impact.has(id)) impact.set(id, linkImpact);
      }
    }
    return impact;
  }, [links]);

  // A stopped animation loop does not repaint when its draw callbacks change.
  // Paint one frame for a theme change, then retain the user's paused state.
  useEffect(() => {
    if (!isPhysicsPaused) return;
    const frame = requestAnimationFrame(() => {
      fgRef.current?.resumeAnimation();
      fgRef.current?.pauseAnimation();
    });
    return () => cancelAnimationFrame(frame);
  }, [canvasColor, isPhysicsPaused]);

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

  const loadGraph = useCallback(async (applyLifecycle = false) => {
    if (applyLifecycle) {
      try {
        await clientFetch<ReplayReport>("/api/graph/lifecycle", { method: "POST" });
      } catch (err) {
        // Keep the existing graph usable while a pre-migration API process is restarting.
        console.warn("Graph lifecycle reconciliation was skipped", err);
      }
    }
    return clientFetch<GraphData>("/api/graph")
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
          color: NODE_TYPE_COLORS[n.node_type] || "var(--ink-3)",
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
          valid_from: e.valid_from,
          valid_to: e.valid_to,
          observed_at: e.observed_at,
          superseded_by: e.superseded_by,
          mental_link_id: e.mental_link_id,
          source_document_id: e.source_document_id,
          target_document_id: e.target_document_id,
          source_quote: e.source_quote,
          target_quote: e.target_quote,
          review_revision: e.review_revision,
          assertion_id: e.assertion_id,
          assertion_scope: e.assertion_scope,
          asserting_document_title: e.asserting_document_title,
          candidate_link_id: e.candidate_link_id,
        }));

        setNodes(gNodes);
        setLinks(gLinks);
      })
      .catch((err) => console.error("Failed to fetch graph", err))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    loadGraph(true);
  }, [loadGraph]);

  const reconcileGraph = useCallback(async () => {
    setReconciling(true);
    try {
      await loadGraph(true);
    } finally {
      setReconciling(false);
    }
  }, [loadGraph]);

  const respondToEdge = useCallback(async (edgeId: string, action: "confirm" | "reject") => {
    await clientFetch(`/api/graph/edges/${edgeId}/respond`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ action, reason: "Reviewed in graph workspace" }),
    });
    await loadGraph(false);
  }, [loadGraph]);

  const respondToConcept = useCallback(async (conceptId: string, action: "confirm" | "reject") => {
    await clientFetch(`/api/graph/concepts/${conceptId}/respond`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ action, reason: "Reviewed in graph workspace" }),
    });
    if (action === "reject") setSelId(null);
    await loadGraph(false);
  }, [loadGraph]);

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

  const selectedNode = useMemo(() => visibleNodes.find((n) => n.id === selId), [visibleNodes, selId]);

  const selectedLinks = useMemo(() => visibleLinks.filter(
    (l) => {
      const src = endpointId(l.source);
      const tgt = endpointId(l.target);
      return src === selId || tgt === selId;
    }
  ), [visibleLinks, selId]);

  // Compute connected nodes for highlighting
  const connectedNodes = useMemo(() => {
    const set = new Set<string>();
    if (!hoverNode) return set;
    set.add(hoverNode);
    for (const l of visibleLinks) {
      const srcId = endpointId(l.source);
      const tgtId = endpointId(l.target);
      if (srcId === hoverNode && tgtId) set.add(tgtId);
      if (tgtId === hoverNode && srcId) set.add(srcId);
    }
    return set;
  }, [visibleLinks, hoverNode]);

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

  // Canvas drawing uses resolved CSS colours; labels remain readable in both themes.
  const nodeCanvasObject = useCallback((node: RenderNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
    if (typeof node.x !== "number" || typeof node.y !== "number" || !isFinite(node.x) || !isFinite(node.y)) return;
    const isSel = node.id === selId;
    const isHovered = node.id === hoverNode;
    const isDimmed = hoverNode !== null && !connectedNodes.has(node.id);
    const interactionImpact = interactionNodeImpact.get(node.id);
    const r = Math.sqrt(node.val || 1) * 4.2;
    const fontSize = Math.max(10 / globalScale, 4.5);
    ctx.save();
    ctx.globalAlpha = isDimmed ? 0.20 : 1;
    if (showInteractionChanges && interactionImpact) {
      ctx.beginPath();
      ctx.arc(node.x, node.y, r + 3.5, 0, 2 * Math.PI);
      ctx.strokeStyle = canvasColor(interactionImpact === "active" ? "var(--accent)" : "var(--error)");
      ctx.lineWidth = 1.4 / globalScale;
      ctx.setLineDash([3 / globalScale, 2 / globalScale]);
      ctx.stroke();
      ctx.setLineDash([]);
    }
    if (isSel || isHovered) {
      ctx.beginPath();
      ctx.arc(node.x, node.y, r + (isSel ? 4.5 : 2.5), 0, 2 * Math.PI);
      ctx.save();
      ctx.globalAlpha *= isSel ? 0.11 : 0.06;
      ctx.fillStyle = canvasColor(node.color);
      ctx.fill();
      ctx.restore();
      ctx.strokeStyle = canvasColor(node.color);
      ctx.lineWidth = 1.2 / globalScale;
      ctx.stroke();
    }
    ctx.beginPath();
    ctx.arc(node.x, node.y, r, 0, 2 * Math.PI);
    ctx.fillStyle = canvasColor(node.color);
    ctx.fill();
    ctx.strokeStyle = canvasColor("var(--bg)");
    ctx.lineWidth = isSel ? 2.5 / globalScale : 1.2 / globalScale;
    ctx.stroke();
    if (globalScale > 1.45 || node.nodeType === "document" || isSel || isHovered) {
      ctx.font = `${isSel ? "600" : "500"} ${fontSize}px sans-serif`;
      ctx.textAlign = "center";
      ctx.textBaseline = "top";
      ctx.strokeStyle = canvasColor("var(--bg)");
      ctx.lineWidth = 4 / globalScale;
      ctx.strokeText(node.name, node.x, node.y + r + 5);
      ctx.fillStyle = canvasColor(isSel ? "var(--ink)" : "var(--ink-3)");
      ctx.fillText(node.name, node.x, node.y + r + 5);
    }
    ctx.restore();
  }, [selId, hoverNode, connectedNodes, interactionNodeImpact, showInteractionChanges, canvasColor]);


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
    const isInactive = ["rejected", "archived", "superseded"].includes(link.state);
    const isInteraction = isInteractionLink(link);
    const isCandidate = isCandidateLink(link);

    ctx.save();

    ctx.beginPath();
    ctx.moveTo(src.x, src.y);
    ctx.lineTo(tgt.x, tgt.y);
    // Reviewed links are solid; unreviewed SELAR suggestions and inactive links are dashed (#117).
    ctx.setLineDash(linkDash(link).map((d) => d / globalScale));

    if (isCandidate) {
      ctx.strokeStyle = canvasColor("var(--text-amber)");
      ctx.globalAlpha = isDimmed ? 0.25 : 1;
      ctx.lineWidth = (isSelectedPath || isHoveredPath ? 2.2 : 1.5) / globalScale;
    } else if (isInactive) {
      ctx.strokeStyle = canvasColor(isInteraction ? "var(--error)" : "var(--ink-3)");
      ctx.globalAlpha = isInteraction ? 0.65 : 0.16;
      ctx.lineWidth = (isInteraction ? 1.4 : 0.6) / globalScale;
    } else if (isDimmed) {
      ctx.strokeStyle = canvasColor("var(--rule-2)");
      ctx.globalAlpha = 0.08;
      ctx.lineWidth = 0.5 / globalScale;
    } else if (isInteraction) {
      ctx.strokeStyle = canvasColor("var(--accent)");
      ctx.globalAlpha = 1;
      ctx.lineWidth = (isSelectedPath || isHoveredPath ? 2.4 : 1.5) / globalScale;
    } else if (isSelectedPath || isHoveredPath) {
      ctx.strokeStyle = canvasColor(REL_COLORS[link.relation] || "var(--ink-3)");
      ctx.globalAlpha = 1;
      ctx.lineWidth = 2.0 / globalScale;
    } else {
      ctx.strokeStyle = canvasColor("var(--ink-3)");
      ctx.globalAlpha = 1;
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

      const labelText = `${link.relation.replace(/_/g, " ")}${isCandidate ? " · to review" : ""}`;
      const labelWidth = ctx.measureText(labelText).width;
      // Labels stay opaque even when an inactive edge is subdued.
      ctx.globalAlpha = 1;
      ctx.fillStyle = canvasColor("var(--bg)");
      ctx.fillRect(midX - labelWidth / 2 - 2, midY - fontSize / 2 - 1, labelWidth + 4, fontSize + 2);
      ctx.fillStyle = canvasColor(isCandidate ? "var(--text-amber)" : isInteraction ? (isInactive ? "var(--error)" : "var(--accent)") : (REL_COLORS[link.relation] || "var(--ink-3)"));
      ctx.fillText(labelText, midX, midY);
    }

    ctx.restore();
  }, [selId, hoverNode, canvasColor]);

  // Filter nodes matching search query
  const filteredNodes = useMemo(() => {
    if (!searchQuery.trim()) return [];
    return visibleNodes.filter(n => n.name.toLowerCase().includes(searchQuery.toLowerCase()));
  }, [visibleNodes, searchQuery]);

  const handleSearchSelect = (node: GraphNode) => {
    setSearchQuery("");
    handleNodeClick(node);
  };

  return (
    <div className={`graph ${styles.workspace}`}>

      {/* Controls occupy their own row so they cannot cover the graph or inspector. */}
      <div className={styles.controls}>
        {/* Concept Finder Search Bar */}
        <div style={{ position: "relative" }}>
          <div style={{
            display: "flex", alignItems: "center", gap: 8,
            background: "var(--bg-raised)", backdropFilter: "blur(12px)",
            border: "1px solid var(--rule)", borderRadius: "var(--r-md)",
            padding: "6px 12px", width: 240, boxShadow: "var(--shadow-1)",
            transition: "all 0.2s ease"
          }}>
            <Icon name="search" size={13} style={{ color: "var(--ink-3)" }} />
            <input
              type="text"
              aria-label="Search graph nodes"
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
                type="button"
                aria-label="Clear graph search"
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
              background: "var(--bg-raised)", backdropFilter: "blur(16px)",
              border: "1px solid var(--rule)", borderRadius: "var(--r-md)",
              boxShadow: "var(--shadow-2)", overflow: "hidden", zIndex: 20,
              maxHeight: 200, overflowY: "auto"
            }}>
              {filteredNodes.map((n) => (
                <button type="button"
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
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Toolbar Controls */}
        <div style={{
          display: "flex", gap: 2,
          background: "var(--bg-raised)", backdropFilter: "blur(12px)",
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
          <button onClick={reconcileGraph} disabled={reconciling} title="Reconcile deterministic projections" style={{
            background: "none", border: "none", padding: "6px 8px", cursor: reconciling ? "wait" : "pointer",
            borderRadius: "var(--r-sm)", color: reconciling ? "var(--accent)" : "var(--ink-2)",
            fontFamily: "var(--font-mono)", fontSize: 9
          }} className="hover-bg-2">
            {reconciling ? "syncing…" : "reconcile"}
          </button>
        </div>

        <div className="graph-layer-toggle" aria-label="Graph knowledge layers">
          <label>
            <input type="checkbox" checked={showPdfKnowledge} onChange={(event) => {
              setShowPdfKnowledge(event.target.checked);
              setSelId(null);
            }} />
            <span className="graph-layer-dot pdf" />
            PDF knowledge <small>{provenanceCounts.pdf}</small>
          </label>
          <label>
            <input type="checkbox" checked={showInteractionChanges} onChange={(event) => {
              setShowInteractionChanges(event.target.checked);
              setSelId(null);
            }} />
            <span className="graph-layer-dot interaction" />
            Your changes <small>{provenanceCounts.interaction}</small>
          </label>
          <label title="Links SELAR suggests because both readings name the same concept. Not reviewed by you.">
            <input type="checkbox" checked={showCandidates} onChange={(event) => {
              setShowCandidates(event.target.checked);
              setSelId(null);
            }} />
            <span className="graph-layer-dot candidate" />
            To review <small>{provenanceCounts.candidate}</small>
          </label>
          <span className="graph-layer-key"><i className="active" /> added <i className="rolled-back" /> rolled back <i className="candidate" /> suggested, not reviewed</span>
        </div>
      </div>

      {/* Main Graph Viewport */}
      <div ref={containerRef} className={styles.viewport} style={{
        flex: 1, position: "relative",
        background: "var(--bg)",
        // Premium subtle blueprint grid pattern
        backgroundImage: "radial-gradient(var(--rule-2) 1px, transparent 1px)",
        backgroundSize: "20px 20px"
      }}>
        {loading && (
          <div className="graph-loading" role="status">
            <div>
              <Icon name="spinner" size={24} className="animate-spin" />
              <span>Loading graph…</span>
            </div>
          </div>
        )}

        {!loading && readingNotice && !noticeDismissed && (
          <div role="status" className="graph-reading-notice">
            <strong>{readingNotice.title}</strong>
            <span>{readingNotice.body}</span>
            <button type="button" aria-label="Dismiss explanation" onClick={() => setNoticeDismissed(true)}>
              <Icon name="x" size={11} />
            </button>
          </div>
        )}

        {!loading && visibleNodes.length === 0 && (
          <div className="graph-empty">
            <EmptyState compact illustration="graph" title={nodes.length === 0 ? "No mental models mapped yet" : "No graph layer selected"}>
              {nodes.length === 0
                ? "Process documents to create evidence-backed claims, concepts, assumptions, and open questions."
                : "Turn on PDF knowledge, Your changes or To review to inspect that layer."}
            </EmptyState>
          </div>
        )}

        {!loading && visibleNodes.length > 0 && (
          <ForceGraph2D<GraphNode, GraphLinkMetadata>
            ref={fgRef}
            width={dimensions.width}
            height={dimensions.height}
            graphData={graphData}
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
              if (["rejected", "archived", "superseded"].includes(link.state)) return 0;
              return isDirectPath ? 3 : 0.8;
            }}
            linkDirectionalParticleSpeed={0.006}
            linkDirectionalParticleWidth={(link) => {
              const srcId = endpointId(link.source);
              const tgtId = endpointId(link.target);
              return (srcId === selId || tgtId === selId) ? 2.2 : 1.2;
            }}
            linkDirectionalParticleColor={(link) => canvasColor(isCandidateLink(link) ? "var(--text-amber)" : isInteractionLink(link) ? "var(--accent)" : (REL_COLORS[link.relation] || "var(--ink-3)"))}
          />
        )}

        {/* Legend Panel (Floating bottom-left) */}
        <div className={styles.legend} style={{
          position: "absolute", bottom: 16, left: 16, display: "flex", flexDirection: "column", gap: 6,
          background: "var(--bg-raised)", backdropFilter: "blur(12px)",
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
      <aside aria-label="Node details" className={styles.details} style={{
        borderLeft: "1px solid var(--rule)", background: "var(--bg-raised)",
        backdropFilter: "blur(20px)", display: "flex", flexDirection: "column", padding: "24px 20px",
        overflowY: "auto", zIndex: 10, boxShadow: "var(--shadow-1)"
      }}>
        {selectedNode ? (
          <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>

            {/* Header Badge */}
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12 }}>
              <span style={{
                fontSize: 9, fontFamily: "var(--font-mono)", fontWeight: 600,
                color: selectedNode.color, background: "var(--bg)",
                border: `1px solid ${selectedNode.color}`,
                borderRadius: 4, padding: "2px 8px", textTransform: "uppercase"
              }}>
                {selectedNode.nodeType.replace(/_/g, " ")} · {selectedNode.state}
              </span>
              <span style={{ flex: 1 }} />
              <button
                aria-label="Close node details"
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

            {selectedNode.nodeType === "concept" && selectedNode.state === "candidate" && (
              <div style={{
                display: "grid", gap: 9, padding: 12, marginBottom: 20,
                border: "1px solid var(--rule-2)", borderRadius: "var(--r-md)",
                background: "var(--bg)"
              }}>
                <span style={{ fontSize: 10, lineHeight: 1.45, color: "var(--ink-3)" }}>
                  This concept was discovered from cited chat evidence. Explore its source passages as learning material.
                </span>
                <div style={{ display: "flex", gap: 7 }}>
                  <p>Source-grounded candidate concept; not evidence of mastery or established knowledge.</p>
                  <button onClick={() => respondToConcept(selectedNode.id, "reject")} style={{
                    border: "1px solid var(--error)", borderRadius: 4,
                    background: "transparent", color: "var(--error)", padding: "6px 10px", cursor: "pointer", fontSize: 10
                  }}>Reject</button>
                </div>
              </div>
            )}

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
                        color: relColor, background: "var(--bg)",
                        border: `1px solid ${relColor}`,
                        borderRadius: 10, padding: "1px 6px", textTransform: "uppercase"
                      }}>
                        {l.relation.replace(/_/g, " ")}
                      </span>
                      <span style={{ flex: 1 }} />
                      <span style={{
                        fontSize: 8, fontFamily: "var(--font-mono)", textTransform: "uppercase",
                        color: ["rejected", "archived", "superseded"].includes(l.state) ? "var(--error)" : "var(--accent)"
                      }}>
                        {l.state}
                      </span>
                      <span style={{ fontSize: 11, color: "var(--ink-4)", fontFamily: "var(--font-mono)" }}>
                        {isOutgoing ? "outgoing →" : "← incoming"}
                      </span>
                    </div>
                    <button type="button" onClick={() => {
                      if (peerId) setSelId(peerId);
                      if (fgRef.current && peer) fgRef.current.centerAt(peer.x, peer.y, 600);
                    }} style={{ textAlign: "left", fontSize: 13, fontWeight: 600, color: "var(--ink-2)" }}>
                      Focus {peer?.name || "related node"}
                    </button>
                    {l.created_via === "deterministic_chat" && (
                      <span style={{ color: "var(--accent)", fontFamily: "var(--font-mono)", fontSize: 9, fontWeight: 600 }}>
                        Adapted from grounded chat · {Math.round((l.confidence || 0) * 100)}% confidence
                      </span>
                    )}
                    {l.mental_link_id && <span style={{ color: "var(--ink-4)", fontFamily: "var(--font-mono)", fontSize: 9 }}>
                      Learner-reviewed exact concept overlap; human note does not establish another relation.
                    </span>}
                    {l.assertion_id && <AssertionProvenance link={l} />}
                    {isCandidateLink(l) && (
                      <div onClick={(event) => event.stopPropagation()} className="graph-candidate-card">
                        <span style={{ color: "var(--text-amber)", fontFamily: "var(--font-mono)", fontSize: 9, fontWeight: 600 }}>
                          Prompt for reflection · check both sources · not evidence of mastery or an established relationship
                        </span>
                        {l.source_quote && <blockquote>{l.source_quote}</blockquote>}
                        {l.target_quote && <blockquote>{l.target_quote}</blockquote>}
                        {candidateReviewURL(l) && <Link href={candidateReviewURL(l)!}>Compare source passages in the Reader</Link>}
                      </div>
                    )}
                    {l.created_via !== "deterministic_chat" && !l.mental_link_id && !l.assertion_id && !isCandidateLink(l) && (
                      <span style={{ color: "var(--ink-4)", fontFamily: "var(--font-mono)", fontSize: 9, fontWeight: 600 }}>
                        PDF-derived knowledge · {Math.round((l.confidence || 0) * 100)}% confidence
                      </span>
                    )}
                    {l.explanation && (
                      <span style={{ color: "var(--ink-4)", fontSize: 10, lineHeight: 1.45 }}>
                        {l.explanation}
                      </span>
                    )}
                    {l.mental_link_id && l.source_document_id && <div onClick={(event) => event.stopPropagation()}>
                      <span>Reviewed assertion · revision {l.review_revision} · two-sided source support</span>
                      <blockquote>{l.source_quote}</blockquote><blockquote>{l.target_quote}</blockquote>
                      <Link href={`/reader?docId=${encodeURIComponent(l.source_document_id)}&linkId=${encodeURIComponent(l.mental_link_id)}`}>Open reviewed assertion in reader</Link>
                    </div>}
                    {l.valid_to && (
                      <span style={{ color: "var(--error)", fontFamily: "var(--font-mono)", fontSize: 9 }}>
                        No longer active since {new Date(l.valid_to).toLocaleDateString()}
                      </span>
                    )}
                    {(l.state === "candidate" || l.state === "supported") && l.created_via === "deterministic_chat" && (
                      <div style={{ display: "flex", gap: 6, marginTop: 2 }} onClick={(event) => event.stopPropagation()}>
                        <span>Historical chat suggestion; no relationship is established by co-citation.</span>
                        <button onClick={() => respondToEdge(l.id, "reject")} style={{
                          border: "1px solid var(--error)", borderRadius: 4, background: "transparent",
                          color: "var(--error)", padding: "4px 8px", cursor: "pointer", fontSize: 9
                        }}>Reject</button>
                      </div>
                    )}
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
