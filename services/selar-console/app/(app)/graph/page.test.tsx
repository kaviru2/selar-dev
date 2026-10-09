import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import GraphPage from "./page";

const api = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api", () => ({ clientFetch: api }));
vi.mock("next/dynamic", () => ({ default: () => function GraphStub({ graphData, onNodeClick }: { graphData: { nodes: { id: string; name: string }[] }; onNodeClick: (node: unknown) => void }) {
  return <button onClick={() => onNodeClick(graphData.nodes[0])}>Select source</button>;
} }));
vi.mock("next/link", () => ({ default: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a> }));
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks(); document.body.innerHTML = ""; });

it("offers source checks rather than knowledge promotion and retains explicit graph rejection", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
  api.mockImplementation(async (url: string) => url === "/api/graph" ? {
    nodes: [
      { id: "a", name: "Source", node_type: "concept", state: "candidate", created_at: "2026-01-01" },
      { id: "b", name: "Other", node_type: "concept", state: "active", created_at: "2026-01-01" },
    ], edges: [
      { id: "candidate", source: "a", target: "b", relation: "concept_overlap", state: "candidate", created_via: "candidate_link", candidate_link_id: "link", source_document_id: "doc", source_quote: "Exact source", target_quote: "Exact target" },
      { id: "chat", source: "a", target: "b", relation: "related_to", state: "candidate", created_via: "deterministic_chat" },
    ],
  } : {});
  const container = document.createElement("div"); document.body.append(container);
  const root = createRoot(container);
  try {
    await act(async () => root.render(<GraphPage />));
    await act(async () => { (Array.from(container.querySelectorAll("button")).find(b => b.textContent === "Select source")!).click(); });
    expect(container.textContent).toContain("Prompt for reflection");
    expect(container.textContent).toContain("not evidence of mastery");
    expect(container.textContent).not.toMatch(/until you decide|Compare and decide/);
    const link = Array.from(container.querySelectorAll("a")).find(a => a.textContent === "Compare source passages in the Reader");
    expect(link?.getAttribute("href")).toContain("linkId=link");
    expect(container.textContent).toContain("Exact source");
    expect(container.textContent).toContain("Exact target");
    const rejects = Array.from(container.querySelectorAll("button")).filter(b => b.textContent === "Reject");
    expect(rejects).toHaveLength(2);
    await act(async () => rejects[1].click());
    expect(api).toHaveBeenCalledWith("/api/graph/edges/chat/respond", expect.objectContaining({ method: "POST", body: expect.stringContaining('"action":"reject"') }));
  } finally { await act(async () => root.unmount()); }
});
