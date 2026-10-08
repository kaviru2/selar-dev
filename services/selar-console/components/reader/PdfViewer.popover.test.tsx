import { act, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LinkSuggestion } from "../../lib/api";

// jsdom cannot render PDFs: Document reports a 2-page document and Page is a stub.
// What is under test is the suggestion popover (ported from PdfCanvas, #96/#102).
vi.mock("react-pdf", () => ({
  pdfjs: { GlobalWorkerOptions: {} },
  Document: function MockDocument({ children, onLoadSuccess }: { children: React.ReactNode; onLoadSuccess?: (pdf: unknown) => void }) {
    useEffect(() => {
      onLoadSuccess?.({
        numPages: 2,
        getPage: async () => ({ getViewport: () => ({ width: 600, height: 800 }), getTextContent: async () => ({ items: [] }) }),
      });
    }, [onLoadSuccess]);
    return <div>{children}</div>;
  },
  Page: () => <div data-testid="pdf-page" />,
}));
vi.mock("react-pdf/dist/Page/AnnotationLayer.css", () => ({}));
vi.mock("react-pdf/dist/Page/TextLayer.css", () => ({}));
vi.mock("./reader.css", () => ({}));

const { default: PdfViewer } = await import("./PdfViewer");

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

const button = (name: RegExp) => Array.from(container.querySelectorAll("button")).find((node) => name.test(node.textContent || "")) as HTMLButtonElement | undefined;
const settle = () => act(async () => { for (let i = 0; i < 8; i++) await new Promise((resolve) => setTimeout(resolve, 0)); });
const dialog = () => container.querySelector('[role="dialog"]');

function viewer(extra: Partial<Parameters<typeof PdfViewer>[0]> = {}) {
  return (
    <PdfViewer
      docId="doc-a" initialPage={1} zoom={1} onZoomChange={() => {}} navRequest={null}
      annotationsOn={false} suggestionsOn suggestions={[match]} annotations={[]}
      showThumbnails={false} onToggleThumbnails={() => {}} onStateChange={() => {}}
      onRespondSuggestion={respond} onOpenSuggestionTarget={() => {}}
      {...extra}
    />
  );
}

async function openCard() {
  await act(async () => root.render(viewer()));
  await settle();
  const mark = container.querySelector(".rd-evidence") as HTMLDivElement;
  expect(mark, "evidence mark").toBeTruthy();
  // Marks are pointer-transparent; keyboard activation is the accessible path.
  await act(async () => { mark.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })); });
  await settle();
  expect(dialog()).toBeTruthy();
}

beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  vi.stubGlobal("ResizeObserver", class {
    constructor(private callback: () => void) {}
    observe() { this.callback(); }
    disconnect() {}
  });
  Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get() { return 800; } });
  Object.defineProperty(HTMLElement.prototype, "clientHeight", { configurable: true, get() { return 900; } });
  container = document.createElement("div"); document.body.append(container); root = createRoot(container);
  respond = vi.fn(async () => true);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
  delete (HTMLElement.prototype as { clientWidth?: number }).clientWidth;
  delete (HTMLElement.prototype as { clientHeight?: number }).clientHeight;
});

describe("PdfViewer passage-match popover", () => {
  it("never offers to accept a similarity-only match and explains why", async () => {
    await openCard();
    expect(button(/accept|confirm/i)).toBeUndefined();
    expect(container.textContent).toMatch(/similarity only/i);
    expect(container.textContent).toMatch(/not a confirmed relation/i);
  });

  it("dismisses a similarity-only match with a rejected response", async () => {
    await openCard();
    await act(async () => { button(/dismiss/i)!.click(); });
    await settle();
    expect(respond).toHaveBeenCalledWith(match.id, "rejected");
    expect(dialog()).toBeNull();
  });

  it("shows the retry error when dismissal fails", async () => {
    respond = vi.fn(async () => false);
    await openCard();
    await act(async () => { button(/dismiss/i)!.click(); });
    await settle();
    expect(container.textContent).toMatch(/Could not save this response/);
  });

  it("closes on Escape", async () => {
    await openCard();
    await act(async () => { window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" })); });
    expect(dialog()).toBeNull();
  });

  it("closes when another document is opened", async () => {
    await openCard();
    await act(async () => root.render(viewer({ docId: "doc-b", suggestions: [] })));
    await settle();
    expect(dialog()).toBeNull();
  });

  it("labels the page controls used by the end-to-end tests", async () => {
    await act(async () => root.render(viewer()));
    await settle();
    expect(container.querySelector('button[aria-label="Previous page"]')).toBeTruthy();
    expect(container.querySelector('button[aria-label="Next page"]')).toBeTruthy();
    expect(container.querySelector(".page-indicator")?.textContent).toBe("1 / 2");
    expect(container.querySelectorAll(".pdf-page-container").length).toBeGreaterThan(0);
  });
});
