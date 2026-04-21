// page.tsx — Graph visualization view.
// Renders the concept node-link diagram fetched from the Go API.

"use client";

import { useState, useEffect } from "react";
import { Icon } from "@/components/ui/Icon";
import { clientFetch, type Concept, type ConceptEdge } from "@/lib/api";

type Node = Concept & {
  x?: number;
  y?: number;
};
type Link = ConceptEdge & {
  source?: Node;
  target?: Node;
};

// Simple force-directed layout approximation based on the schema design
export default function GraphPage() {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    clientFetch<{ nodes: Concept[]; edges: ConceptEdge[] }>("/api/graph")
      .then((data) => {
        // Simple circular layout fallback since no d3 yet
        const dNodes = data.nodes.map((n, i) => {
          const angle = (i / data.nodes.length) * 2 * Math.PI;
          const r = 200;
          return {
            ...n,
            x: 400 + r * Math.cos(angle),
            y: 300 + r * Math.sin(angle),
          };
        });

        const dLinks = data.edges.map((e) => {
          return {
            ...e,
            source: dNodes.find((n) => n.id === e.source_concept_id),
            target: dNodes.find((n) => n.id === e.target_concept_id),
          };
        });

        setNodes(dNodes);
        setLinks(dLinks);
      })
      .catch((err) => console.error("Failed to fetch graph", err))
      .finally(() => setLoading(false));
  }, []);

  const [selId, setSelId] = useState<string | null>(null);
  const selectedNode = nodes.find((n) => n.id === selId);

  return (
    <div className="graph" style={{ display: "flex", width: "100%", height: "100%" }}>
      {/* SVG Canvas */}
      <div style={{ flex: 1, position: "relative" }}>
        <svg
          style={{ width: "100%", height: "100%", cursor: "grab" }}
          viewBox="0 0 800 600"
        >
          {loading && (
             <text x="400" y="300" textAnchor="middle" fill="var(--ink-4)" fontSize="14px">Loading Graph...</text>
          )}

          {!loading && nodes.length === 0 && (
             <text x="400" y="300" textAnchor="middle" fill="var(--ink-4)" fontSize="14px">Your Knowledge Graph is empty.</text>
          )}

          <g stroke="var(--rule)" strokeWidth="1.5">
            {links.map((link) => {
               if (!link.source || !link.target) return null;
               return (
                <line
                  key={link.id}
                  x1={link.source.x}
                  y1={link.source.y}
                  x2={link.target.x}
                  y2={link.target.y}
                  strokeDasharray={link.created_via === "ai_suggested" ? "4 4" : "none"}
                />
               );
            })}
          </g>

          <g>
            {nodes.map((node) => {
               const isSel = node.id === selId;
               return (
                <g
                  key={node.id}
                  transform={`translate(${node.x},${node.y})`}
                  onClick={() => setSelId(node.id)}
                  style={{ cursor: "pointer" }}
                >
                  <circle
                    r="6"
                    fill={isSel ? "var(--accent)" : "var(--bg-3)"}
                    stroke={isSel ? "var(--bg)" : "var(--ink-4)"}
                    strokeWidth="2"
                    style={{ transition: "all 0.15s" }}
                  />
                  <text
                    y="20"
                    textAnchor="middle"
                    fill={isSel ? "var(--ink)" : "var(--ink-3)"}
                    fontSize="11"
                    fontFamily="var(--font-sans)"
                    style={{ transition: "all 0.15s" }}
                  >
                    {node.name}
                  </text>
                </g>
               );
            })}
          </g>
        </svg>

        <div style={{ position: "absolute", top: 16, left: 16, display: "flex", gap: 8 }}>
           <div className="kbd-hint" style={{ background: "var(--bg)", border: "1px solid var(--rule)" }}>
             <span className="dot" style={{ background: "var(--accent)", width: 8, height: 8 }} /> User confirmed
           </div>
           <div className="kbd-hint" style={{ background: "var(--bg)", border: "1px solid var(--rule)" }}>
             <span className="dot" style={{ background: "var(--ink-4)", width: 8, height: 8 }} /> AI suggested
           </div>
        </div>
      </div>

      {/* Pane */}
      <aside className="concept-pane" style={{
        width: 300, borderLeft: "1px solid var(--rule)", background: "var(--bg)",
        display: "flex", flexDirection: "column", padding: 24
      }}>
        {selectedNode ? (
          <>
            <h2 style={{ fontSize: 20, margin: "0 0 12px", fontWeight: 600 }}>{selectedNode.name}</h2>
            <p style={{ fontSize: "var(--t-sm)", color: "var(--ink-3)", lineHeight: 1.5, flex: 1 }}>
              {selectedNode.description || "No description generated yet for this concept."}
            </p>

            <div style={{ padding: 16, background: "var(--bg-2)", borderRadius: "var(--r-sm)", marginTop: 24 }}>
              <div style={{ fontSize: 10, fontFamily: "var(--font-mono)", color: "var(--ink-4)", marginBottom: 8, textTransform: "uppercase" }}>
                Connected Mentions
              </div>
              <ul style={{ margin: 0, padding: 0, listStyle: "none", display: "flex", flexDirection: "column", gap: 12 }}>
                {links.filter(l => l.source_concept_id === selId || l.target_concept_id === selId).map(l => (
                   <li key={l.id} style={{ fontSize: "var(--t-sm)", display: "flex", gap: 8, alignItems: "flex-start" }}>
                      <Icon name="tag" size={12} style={{ color: "var(--accent-2)", marginTop: 2 }} />
                      <div>
                        Relation: {l.relation}<br/>
                        <span style={{ fontSize: 10, color: "var(--ink-4)" }}>{l.created_via}</span>
                      </div>
                   </li>
                ))}
              </ul>
            </div>
          </>
        ) : (
          <div style={{ margin: "auto", textAlign: "center", color: "var(--ink-4)", fontSize: "var(--t-sm)" }}>
            Select a node to inspect its ontology.
          </div>
        )}
      </aside>
    </div>
  );
}
