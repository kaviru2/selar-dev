import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewAssertion } from "./ReviewAssertion";
import type { MentalLinkReviewPreview } from "../lib/api";

const witness = {
  id: "review-a", user_id: "owner", source_model_id: "model-new", target_model_id: "model-prior",
  similarity: 0.8, confidence: 0.8, bridge_explanation: "Shared term", created_via: "verified",
  suggested_at: "2026-09-25T00:00:00Z", revision: 0, status: "candidate", link_type: "concept_overlap",
  source_document_id: "new-doc", target_document_id: "prior-doc",
  source_document_title: "New", target_document_title: "Prior",
  source_quote: "The model uses gradient descent.", target_quote: "Gradient descent updates parameters.",
  source_locator: { page: 2 }, target_locator: { page: 3 }, history: [],
} as MentalLinkReviewPreview;
let container: HTMLDivElement;
let root: Root;
let actReview: ReturnType<typeof vi.fn>;
let fetchSpy: ReturnType<typeof vi.fn>;
let storageSpy: ReturnType<typeof vi.spyOn>;
const render = async (preview = witness, documentId = "new-doc") => {
  await act(async () => root.render(<ReviewAssertion preview={preview} documentId={documentId} label="" reason="" busy={false} onLabel={() => {}} onReason={() => {}} onAct={actReview} />));
};
const button = (name: RegExp) => Array.from(container.querySelectorAll("button")).find(node => name.test(node.textContent || "")) as HTMLButtonElement | undefined;
const type = async (field: HTMLTextAreaElement, value: string) => {
  await act(async () => {
    // React's change tracking requires the native setter for a synthetic input event.
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(field, value);
    field.dispatchEvent(new Event("input", { bubbles: true }));
  });
};

beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div"); document.body.append(container); root = createRoot(container);
  actReview = vi.fn(); fetchSpy = vi.fn(); vi.stubGlobal("fetch", fetchSpy);
  storageSpy = vi.spyOn(Storage.prototype, "setItem");
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); storageSpy.mockRestore(); vi.unstubAllGlobals(); });

describe("optional reflection at the verified review seam", () => {
  it("lets a learner draft an explanation without saving or submitting it", async () => {
    await render();
    const field = container.querySelector('textarea[aria-label="Your explanation of how the claims connect"]') as HTMLTextAreaElement;
    expect(field).toBeTruthy();
    expect(container.textContent).toMatch(/optional.*not saved/i);
    await type(field, "They describe the same optimization process in different terms.");
    expect(field.value).toContain("optimization process");
    expect(container.textContent).toContain(witness.source_quote);
    expect(container.textContent).toContain(witness.target_quote);
    expect(actReview).not.toHaveBeenCalled();
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(storageSpy).not.toHaveBeenCalled();
  });
  it("offers optional recall before revisiting these quotes, without claiming blinded assessment", async () => {
    await render();
    expect(button(/try recalling/i)).toBeTruthy();
    await act(async () => button(/try recalling/i)?.click());
    expect(container.textContent).not.toContain(witness.source_quote);
    expect(container.textContent).not.toContain(witness.target_quote);
    expect(container.textContent).toMatch(/quotes may still be visible elsewhere/i);
    const recall = container.querySelector('textarea[aria-label="Your optional recall of the relationship"]') as HTMLTextAreaElement;
    expect(recall).toBeTruthy();
    expect(document.activeElement).toBe(recall);
    await type(recall, "Gradient descent is the shared process.");
    expect(recall.value).toContain("shared process");
    expect(container.querySelector('[role="status"]')?.textContent).toMatch(/quotes.*hidden.*this review/i);
    await act(async () => button(/revisit source quotes/i)?.click());
    expect(container.textContent).toContain(witness.source_quote);
    expect(container.textContent).toContain(witness.target_quote);
    expect((container.querySelector('textarea[aria-label="Your optional recall of the relationship"]') as HTMLTextAreaElement).value).toContain("shared process");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Source quotes for comparison");
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(storageSpy).not.toHaveBeenCalled();
    expect(actReview).not.toHaveBeenCalled();
  });
  it("clears both drafts when the candidate or reader document changes and on unmount", async () => {
    await render();
    const explain = () => container.querySelector('textarea[aria-label="Your explanation of how the claims connect"]') as HTMLTextAreaElement;
    await type(explain(), "Private explanation for A");
    await act(async () => button(/try recalling/i)?.click());
    await type(container.querySelector('textarea[aria-label="Your optional recall of the relationship"]') as HTMLTextAreaElement, "Private recall for A");
    await render({ ...witness, id: "review-b" });
    expect(explain().value).toBe("");
    expect(container.querySelector('textarea[aria-label="Your optional recall of the relationship"]')).toBeNull();
    await type(explain(), "Private explanation for B");
    await render({ ...witness, id: "review-b" }, "prior-doc");
    expect(explain().value).toBe("");
    await type(explain(), "Private explanation before unmount");
    await act(async () => root.render(null));
    await render({ ...witness, id: "review-b" }, "prior-doc");
    expect(explain().value).toBe("");
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(storageSpy).not.toHaveBeenCalled();
  });
  it("provides a helpful empty drafting state and explicit discard without affecting human review", async () => {
    await render();
    const explanation = container.querySelector('textarea[aria-label="Your explanation of how the claims connect"]') as HTMLTextAreaElement;
    expect(explanation.placeholder).toMatch(/compare.*two quotes/i);
    await type(explanation, "This is my private interpretation, not a source fact.");
    await act(async () => button(/discard draft/i)?.click());
    expect(explanation.value).toBe("");
    expect(document.activeElement).toBe(explanation);
    expect(container.querySelector('[role="status"]')?.textContent).toMatch(/draft discarded/i);
    expect(actReview).not.toHaveBeenCalled();
  });
  it.each(["source_quote", "target_quote"] as const)("fails closed when %s is missing", async (missing) => {
    await render({ ...witness, [missing]: " " });
    expect(container.querySelector('[role="alert"]')?.textContent).toMatch(/two exact source quotes.*unavailable/i);
    expect(container.querySelector("textarea")).toBeNull();
    expect(button(/confirm|reject|correct/i)).toBeUndefined();
    expect(container.querySelector('a[href*="reader"]')).toBeNull();
    expect(actReview).not.toHaveBeenCalled();
  });
  it("keeps evidence-derived relation and human governance separate from private draft text", async () => {
    await render();
    const explanation = container.querySelector('textarea[aria-label="Your explanation of how the claims connect"]') as HTMLTextAreaElement;
    await type(explanation, "Claim extension, perhaps? This is not an authoritative label.");
    expect(container.textContent).toContain("Review concept overlap");
    expect(button(/^confirmed$/i)).toBeTruthy();
    await act(async () => button(/^confirmed$/i)?.click());
    expect(actReview).toHaveBeenCalledOnce();
    expect(actReview).toHaveBeenCalledWith("confirmed");
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(storageSpy).not.toHaveBeenCalled();
  });
  it.each(["source_locator", "target_locator"] as const)("fails closed without a %s", async (missing) => {
    await render({ ...witness, [missing]: undefined });
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("locations are unavailable");
    expect(container.querySelector("textarea")).toBeNull();
    expect(button(/^confirmed$/i)).toBeUndefined();
  });
});
