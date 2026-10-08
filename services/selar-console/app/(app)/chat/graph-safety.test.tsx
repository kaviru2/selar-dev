import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import ChatPage from "./page";

const clientFetch = vi.fn();
vi.mock("@/lib/api", () => ({ clientFetch: (...args: unknown[]) => clientFetch(...args) }));

afterEach(() => { vi.clearAllMocks(); document.body.innerHTML = ""; });

it("keeps citations and answer feedback visible without calling a rating relationship proof", async () => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  HTMLElement.prototype.scrollTo = vi.fn();
  clientFetch.mockImplementation(async (path: string) => path === "/api/chat/threads"
    ? [{ id: "thread", title: "Example" }]
    : [{ id: "answer", role: "assistant", status: "complete", content: "A cited answer", citations: [
      { id: "citation", chunk_id: "chunk", document_id: "document", document_title: "Source", source_type: "pdf", page: 1, rank: 1, quote: "A source passage" },
    ], feedback: [{ action: "helpful" }], graph_update: {
      concepts_created: 1, concepts_reinforced: 1, links_observed: 2, links_promoted: 1,
    } }]);
  const container = document.createElement("div"); document.body.append(container);
  const root = createRoot(container);
  try {
    await act(async () => { root.render(<ChatPage />); });
    await act(async () => { await Promise.resolve(); });
    expect(container.textContent).toContain("Source");
    expect(container.textContent).toContain("Saved");
    expect(container.textContent).toContain("candidate");
    expect(container.textContent).not.toMatch(/feedback-approved|reinforced|candidate relationships observed|promoted to supported/i);
  } finally { await act(async () => root.unmount()); }
});

it("shows retracted chat evidence as withdrawn instead of linking it into the graph", async () => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  HTMLElement.prototype.scrollTo = vi.fn();
  clientFetch.mockImplementation(async (path: string) => path === "/api/chat/threads"
    ? [{ id: "thread", title: "Example" }]
    : [{ id: "answer", role: "assistant", status: "complete", content: "A cited answer", citations: [],
      feedback: [{ action: "helpful" }, { action: "unhelpful" }], graph_update: {
        concepts_created: 0, concepts_reinforced: 2, links_observed: 0, links_promoted: 0, retracted: true,
      } }]);
  const container = document.createElement("div"); document.body.append(container);
  const root = createRoot(container);
  try {
    await act(async () => { root.render(<ChatPage />); });
    await act(async () => { await Promise.resolve(); });
    expect(container.textContent).toContain("withdrawn");
    expect(container.textContent).not.toContain("associated with citations");
    expect(container.querySelector('a[href="/graph"].chat-graph-update')).toBeNull();
  } finally { await act(async () => root.unmount()); }
});
