import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LinkSuggestion } from "../lib/api";

// jsdom cannot render PDFs; the passage-card popover is what is under test.
vi.mock("react-pdf", () => ({
  pdfjs: { GlobalWorkerOptions: {} },
  Document: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Page: () => <div data-testid="pdf-page" />,
}));
vi.mock("react-pdf/dist/Page/AnnotationLayer.css", () => ({}));
vi.mock("react-pdf/dist/Page/TextLayer.css", () => ({}));

const { default: PdfCanvas } = await import("./PdfCanvas");

const match = {
  id: "0cdac324-synthetic", user_id: "owner", source_chunk_id: "c1", target_chunk_id: "c2",
  similarity: 0.74, relation: "unclassified", status: "pending", user_label: null,
  src_text: "Compare our solution with alternative solutions.", tgt_text: "Experiments compare against baselines.",
  src_document_id: "doc-a", tgt_document_id: "doc-b", src_doc: "Synthetic A.pdf", tgt_doc: "Synthetic B.pdf",
  src_page: 1, tgt_page: 18, summary: "", suggested_at: "2026-10-08T00:00:00Z", responded_at: null,
  src_bboxes: [{ x: 0.1, y: 0.1, w: 0.3, h: 0.05 }],
} as LinkSuggestion;

let container: HTMLDivElement;
let root: Root;
let respond: ReturnType<typeof vi.fn>;

const buttons = () => Array.from(container.querySelectorAll("button"));
const button = (name: RegExp) => buttons().find((node) => name.test(node.textContent || "")) as HTMLButtonElement | undefined;
const settle = () => act(async () => { for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0)); });

async function openCard(suggestion: LinkSuggestion) {
  await act(async () => root.render(
    <PdfCanvas docId="doc-a" zoom={1} pageNumber={1} annotationsOn={false} suggestionsOn
      suggestions={[suggestion]} onPageLoad={() => {}} onRespondSuggestion={respond} onOpenSuggestionTarget={() => {}} />,
  ));
  const mark = container.querySelector(".suggestion-mark") as HTMLDivElement;
  expect(mark, "suggestion mark").toBeTruthy();
  await act(async () => { mark.click(); });
  await settle();
  expect(container.querySelector('[role="dialog"]')).toBeTruthy();
}

beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div"); document.body.append(container); root = createRoot(container);
  respond = vi.fn(async () => true);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

describe("PdfCanvas passage-match popover", () => {
  it("never offers to accept a similarity-only match and explains why", async () => {
    await openCard(match);
    expect(button(/accept|confirm/i)).toBeUndefined();
    expect(container.textContent).toMatch(/similarity only/i);
    expect(container.textContent).toMatch(/not a confirmed relation/i);
  });

  it("dismisses a similarity-only match with a rejected response", async () => {
    await openCard(match);
    const dismiss = button(/dismiss/i);
    expect(dismiss, "dismiss button").toBeTruthy();
    await act(async () => { dismiss!.click(); });
    await settle();
    expect(respond).toHaveBeenCalledTimes(1);
    expect(respond).toHaveBeenCalledWith(match.id, "rejected");
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it("still shows the retry error when dismissal fails", async () => {
    respond = vi.fn(async () => false);
    await openCard(match);
    await act(async () => { button(/dismiss/i)!.click(); });
    await settle();
    expect(container.textContent).toMatch(/Could not save this response/);
  });

  it("closes on Escape", async () => {
    await openCard(match);
    await act(async () => { window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" })); });
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it("closes when the reader moves to another page", async () => {
    await openCard(match);
    await act(async () => root.render(
      <PdfCanvas docId="doc-a" zoom={1} pageNumber={2} annotationsOn={false} suggestionsOn
        suggestions={[match]} onPageLoad={() => {}} onRespondSuggestion={respond} onOpenSuggestionTarget={() => {}} />,
    ));
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });
});
