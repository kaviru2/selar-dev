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
    expect(container.querySelector('button[aria-label="Helpful"]')?.getAttribute("aria-pressed")).toBe("true");
    expect(container.textContent).toContain("candidate");
    expect(container.textContent).toContain("never shows that concepts are related");
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
    expect(container.textContent).not.toContain("associated with");
    expect(container.querySelector('a[href="/graph"]')).toBeNull();
  } finally { await act(async () => root.unmount()); }
});

it("routes a graph-command answer to a source-scoped proposal form, not to a library answer", async () => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  HTMLElement.prototype.scrollTo = vi.fn();
  clientFetch.mockImplementation(async (path: string) => path === "/api/chat/threads"
    ? [{ id: "thread", title: "Example" }]
    : [
      { id: "a1", role: "assistant", status: "complete", content: "Comparison answer", citations: [
        { id: "c", chunk_id: "chunk", document_id: "doc", document_title: "Later paper", source_type: "pdf", page: 2, rank: 1, quote: "A later paper reports a modified baseline." },
      ] },
      { id: "q2", role: "user", status: "complete", content: "Update the graph", citations: [] },
      { id: "a2", role: "assistant", status: "complete", model_version: "deterministic-graph-command-boundary-v1",
        content: "No graph change was made, and no preview was created for this request.", citations: [] },
    ]);
  const container = document.createElement("div"); document.body.append(container);
  const root = createRoot(container);
  try {
    await act(async () => { root.render(<ChatPage />); });
    await act(async () => { await Promise.resolve(); });
    const forms = container.querySelectorAll('form[aria-label="Propose a graph correction"]');
    expect(forms.length).toBe(1);
    expect(forms[0].closest("article")?.textContent).toContain("No graph change was made");
    expect(forms[0].textContent).toContain("Later paper");
    // The command-boundary reply gets the proposal form only, not a second feedback surface.
    expect(forms[0].closest("article")?.querySelector('[aria-label="Answer feedback"]')).toBeNull();
    expect(container.querySelectorAll('[aria-label="Answer feedback"]').length).toBe(1);
    expect(clientFetch.mock.calls.every(([, init]) => !init || init.method !== "POST")).toBe(true);
  } finally { await act(async () => root.unmount()); }
});
