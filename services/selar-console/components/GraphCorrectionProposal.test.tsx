import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import type { ChatMessage } from "@/lib/api";
import { GraphCorrectionProposal } from "./GraphCorrectionProposal";

const clientFetch = vi.fn();
vi.mock("@/lib/api", () => ({ clientFetch: (...args: unknown[]) => clientFetch(...args) }));

afterEach(() => { vi.clearAllMocks(); document.body.innerHTML = ""; });

// Fabricated RAC/SagaLLM shape; invented text.
const racQuote = "As a baseline we re-implement a modified SagaLLM and run it on τ²-bench.";
const history: ChatMessage[] = [{
  id: "a1", thread_id: "t", user_id: "u", role: "assistant", status: "complete", content: "…", created_at: "",
  citations: [{ id: "c", chunk_id: "chunk-rac", document_id: "doc-rac", document_title: "RAC paper (fabricated)",
    page: 6, rank: 1, score: 1, quote: racQuote, source_type: "pdf", locator: {} }],
}];

function setValue(element: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, value: string) {
  const proto = Object.getPrototypeOf(element);
  Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(element, value);
  element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }));
}

async function mount(messages: ChatMessage[]) {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  const container = document.createElement("div"); document.body.append(container);
  const root: Root = createRoot(container);
  await act(async () => { root.render(<GraphCorrectionProposal messages={messages} sourceMessageId="graph-cmd" />); });
  return { container, root };
}

it("fails closed without a cited passage and makes no request", async () => {
  const { container, root } = await mount([]);
  expect(container.textContent).toContain("No cited passage");
  expect(container.querySelector("form")).toBeNull();
  expect(clientFetch).not.toHaveBeenCalled();
  await act(async () => root.unmount());
});

it("creates only a proposed assertion and confirms the exact revision previewed", async () => {
  const proposed = { id: "as-1", subject: "modified SagaLLM", predicate: "evaluated_on", object: "τ²-bench",
    asserting_document_id: "doc-rac", asserting_document_title: "RAC paper (fabricated)", scope: "reported_about_other",
    state: "proposed", revision: 1, evidence: [{ chunk_id: "chunk-rac", quote: racQuote }] };
  clientFetch.mockResolvedValueOnce(proposed).mockResolvedValueOnce({ ...proposed, state: "confirmed", revision: 2 });
  const { container, root } = await mount(history);
  const submit = container.querySelector<HTMLButtonElement>('button[type="submit"]')!;
  expect(submit.disabled).toBe(true); // nothing to submit without a passage and claim
  await act(async () => {
    setValue(container.querySelector("select")!, "chunk-rac");
    setValue(container.querySelector<HTMLInputElement>('input[aria-label="Subject"]')!, "modified SagaLLM");
    setValue(container.querySelector<HTMLInputElement>('input[aria-label="Object"]')!, "τ²-bench");
  });
  expect(container.textContent).toContain("Asserting document: RAC paper (fabricated)");
  expect(clientFetch).not.toHaveBeenCalled(); // typing never writes
  await act(async () => { submit.click(); });
  expect(clientFetch).toHaveBeenCalledTimes(1);
  const [path, init] = clientFetch.mock.calls[0];
  expect(path).toBe("/api/research-assertions");
  const body = JSON.parse(init.body);
  expect(body).toMatchObject({ subject: "modified SagaLLM", predicate: "evaluated_on", object: "τ²-bench",
    scope: "reported_about_other", asserting_document_id: "doc-rac", source_message_id: "graph-cmd",
    evidence: [{ chunk_id: "chunk-rac", quote: racQuote }] });
  expect(container.textContent).toContain("not yet in your graph");
  expect(container.textContent).toContain("not that work's own paper");
  const confirm = Array.from(container.querySelectorAll("button")).find((b) => b.textContent?.startsWith("Save sourced assertion"))!;
  await act(async () => { confirm.click(); });
  expect(clientFetch.mock.calls[1][0]).toBe("/api/research-assertions/as-1/respond");
  expect(JSON.parse(clientFetch.mock.calls[1][1].body)).toEqual({ action: "confirm", revision: 1 });
  expect(container.textContent).toContain("Correction saved");
  await act(async () => root.unmount());
});
