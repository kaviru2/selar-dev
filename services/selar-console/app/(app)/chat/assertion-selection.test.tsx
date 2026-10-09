import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import ChatPage from "./page";

const api = vi.fn();
vi.mock("@/lib/api", () => ({ clientFetch: (...args: unknown[]) => api(...args) }));

it("requires explicit document and saved assertion selection; binds the displayed tuple", async () => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  HTMLElement.prototype.scrollTo = vi.fn();
  const assertion = { id: "assertion", asserting_document_id: "doc", asserting_document_title: "Comparison paper",
    subject: "modified SagaLLM", predicate: "evaluated_on", object: "τ²-bench", scope: "reported_about_other",
    subject_qualifier: "modified baseline", revision: 2, state: "confirmed", evidence: [{quote:"A witness",chunk_id:"c"}] };
  api.mockImplementation(async (path: string) => path === "/api/chat/threads" ? [{id:"thread",title:"Example"}]
    : path.startsWith("/api/research-assertions") ? [assertion] : []);
  const container = document.createElement("div"); document.body.append(container);
  const root = createRoot(container);
  try {
    await act(async () => root.render(<ChatPage />));
    const open = Array.from(container.querySelectorAll("button")).find(b => b.textContent === "Browse saved assertions");
    expect(open).toBeDefined();
    await act(async () => open!.click());
    const doc = container.querySelector('select[aria-label="Asserting document"]') as HTMLSelectElement;
    expect(doc.value).toBe("");
    await act(async () => { doc.value="doc"; doc.dispatchEvent(new Event("change",{bubbles:true})); });
    const saved = container.querySelector('select[aria-label="Confirmed saved assertion"]') as HTMLSelectElement;
    expect(saved.value).toBe("");
    await act(async () => { saved.value="assertion"; saved.dispatchEvent(new Event("change",{bubbles:true})); });
    expect(container.textContent).toContain("not proof of entailment");
    expect(container.textContent).toContain("modified baseline");
    const textarea = container.querySelector(".chat-composer textarea") as HTMLTextAreaElement;
    expect(textarea.value).toBe('Show saved assertion: ["modified SagaLLM","evaluated_on","τ²-bench","reported_about_other","","modified baseline",""]');
    await act(async () => container.querySelector(".chat-composer")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));
    const post = api.mock.calls.find(([, init]) => init?.method === "POST");
    expect(JSON.parse(post![1].body)).toEqual({content:textarea.value || 'Show saved assertion: ["modified SagaLLM","evaluated_on","τ²-bench","reported_about_other","","modified baseline",""]',
      assertion_selection:{asserting_document_id:"doc",assertion_id:"assertion",revision:2}});
  } finally { await act(async () => root.unmount()); container.remove(); }
});
