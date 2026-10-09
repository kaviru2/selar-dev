import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConnectionsPanel } from "./ConnectionsPanel";
import type { MentalLinkReviewPreview, MentalModelLink } from "../lib/api";

const link = {
  id: "link-1", user_id: "owner", source_model_id: "m-new", target_model_id: "m-old",
  source_document_id: "new-doc", target_document_id: "old-doc",
  source_document_title: "Study strategies that last", target_document_title: "Week 3 notes: how memory works",
  link_type: "concept_overlap", similarity: 0.7, confidence: 0.65,
  bridge_explanation: "Both readings explicitly discuss Retrieval Practice. Compare their treatment.",
  status: "candidate", created_via: "ai_suggested", suggested_at: "2026-10-01T00:00:00Z", revision: 0,
  source_quote: "Retrieval practice produces more durable learning than rereading.",
  target_quote: "Self-testing beats rereading.",
  source_locator: { page: 1 }, target_locator: { block_index: 0 },
} as MentalModelLink;
const preview = (overrides: Partial<MentalLinkReviewPreview> = {}) => ({ ...link, history: [], ...overrides }) as MentalLinkReviewPreview;

let container: HTMLDivElement;
let root: Root;
let fetchSpy: ReturnType<typeof vi.fn>;
let links: MentalModelLink[];
let reload: ReturnType<typeof vi.fn>;
let currentPreview: MentalLinkReviewPreview;
let storageSpy: ReturnType<typeof vi.spyOn>;

const json = (body: unknown, status = 200) => Promise.resolve(new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }));
const button = (name: RegExp) => Array.from(container.querySelectorAll("button")).find((node) => name.test(node.textContent || "")) as HTMLButtonElement | undefined;
const settle = () => act(async () => { for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0)); });
const click = async (name: RegExp) => { const b = button(name); expect(b, `button ${name}`).toBeTruthy(); await act(async () => { b!.click(); }); await settle(); };
const type = async (field: HTMLTextAreaElement | HTMLInputElement, value: string) => {
  await act(async () => {
    const proto = field instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value")?.set?.call(field, value);
    field.dispatchEvent(new Event("input", { bubbles: true }));
  });
};
const render = async (extra: Partial<Parameters<typeof ConnectionsPanel>[0]> = {}) => {
  await act(async () => root.render(
    <ConnectionsPanel docId="new-doc" links={links} loading={false} mentalModel={null} reloadLinks={reload} {...extra} />,
  ));
  await settle();
};
const respondCalls = () => fetchSpy.mock.calls.filter(([url, init]) => String(url).endsWith("/respond") && init?.method === "POST")
  .map(([, init]) => JSON.parse(init.body));

beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div"); document.body.append(container); root = createRoot(container);
  links = [link];
  currentPreview = preview();
  storageSpy = vi.spyOn(Storage.prototype, "setItem");
  fetchSpy = vi.fn((url: string, init?: RequestInit) => {
    if (url.endsWith("/preview")) return json(currentPreview);
    if (url.endsWith("/respond") && init?.method === "POST") {
      const body = JSON.parse(String(init.body));
      if (body.action === "confirmed") { links = [{ ...link, status: "confirmed", revision: 1 }]; currentPreview = preview({ status: "confirmed", revision: 1 }); }
      if (body.action === "rejected") { links = [{ ...link, status: "rejected", revision: 1 }]; currentPreview = preview({ status: "rejected", revision: 1 }); }
      if (body.action === "rolled_back") { links = [link]; currentPreview = preview({ revision: 2 }); }
      return json({ ok: true });
    }
    return json({});
  });
  vi.stubGlobal("fetch", fetchSpy);
  reload = vi.fn(async () => { await act(async () => root.render(
    <ConnectionsPanel docId="new-doc" links={links} loading={false} mentalModel={null} reloadLinks={reload} />,
  )); return links; });
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); storageSpy.mockRestore(); vi.unstubAllGlobals(); });

describe("guided connections sidebar", () => {
  it("with no linked readings, explains the exact-concept rule instead of blaming processing (#117)", async () => {
    links = [];
    await render();
    const text = container.textContent || "";
    expect(text).toContain("No suggested connections yet");
    expect(text).not.toMatch(/at least two of your readings have been processed/);
    expect(text).toMatch(/both name the same concept/);
    expect(text).toMatch(/Similar passages/);
  });

  it("step 1 states the suggestion in plain words without quotes or a percentage", async () => {
    await render();
    const text = container.textContent || "";
    expect(text).toContain("1 to review · 0 kept");
    expect(text).toContain("Week 3 notes: how memory works");
    expect(text).toContain("Both readings discuss retrieval practice.");
    expect(text).toContain("Moderate overlap");
    expect(text).toMatch(/suggestion, not a fact/i);
    expect(text).not.toContain(link.source_quote!);
    expect(text).not.toContain(link.target_quote!);
    expect(text).not.toMatch(/\d+%/);
    expect(text).not.toMatch(/argument-level|review assertion|revision \d/i);
  });

  it("asks for the learner's explanation before revealing the passages, without saving it", async () => {
    await render();
    await click(/think about this link/i);
    const explain = container.querySelector('textarea[aria-label="Your explanation of how the readings connect"]') as HTMLTextAreaElement;
    expect(explain).toBeTruthy();
    expect(container.textContent).not.toContain(link.source_quote!);
    expect(container.textContent).toMatch(/not graded, not saved/i);
    await type(explain, "Week 3 explains why recall strengthens memory.");
    await click(/show me the passages/i);
    const text = container.textContent || "";
    expect(text).toContain("Week 3 explains why recall strengthens memory.");
    expect(text).toContain(link.source_quote!);
    expect(text).toContain(link.target_quote!);
    expect(text).toContain("p. 1");
    const hrefs = Array.from(container.querySelectorAll("a")).map((a) => a.getAttribute("href"));
    expect(hrefs).toContain("/reader?docId=new-doc&page=1");
    expect(hrefs).toContain("/reader?docId=old-doc&block=0");
    expect(respondCalls()).toHaveLength(0);
    expect(fetchSpy.mock.calls.some(([, init]) => String(init?.body || "").includes("recall strengthens"))).toBe(false);
    expect(storageSpy).not.toHaveBeenCalled();
  });

  it("lets the reader open a compare quote in place, and falls back to the link otherwise", async () => {
    const onOpenWitness = vi.fn((docId: string) => docId === "new-doc");
    await render({ onOpenWitness });
    await click(/think about this link/i);
    await click(/show me the passages/i);
    const [thisReading, other] = Array.from(container.querySelectorAll("figure a")) as HTMLAnchorElement[];
    const clickLink = async (anchor: HTMLAnchorElement) => {
      const event = new MouseEvent("click", { bubbles: true, cancelable: true });
      await act(async () => { anchor.dispatchEvent(event); });
      return event.defaultPrevented;
    };
    expect(await clickLink(thisReading)).toBe(true);
    expect(onOpenWitness).toHaveBeenCalledWith("new-doc", { page: 1 }, link.source_quote);
    expect(await clickLink(other)).toBe(false);
    expect(onOpenWitness).toHaveBeenLastCalledWith("old-doc", { block_index: 0 }, link.target_quote);
    // Still on the compare step: the guided card is not lost.
    expect(container.textContent).toContain("Compare with the sources");
  });

  it("keeps a link with a revision-bound decision and offers undo", async () => {
    await render();
    await click(/think about this link/i);
    await click(/show me the passages/i);
    await click(/yes, keep this link/i);
    expect(respondCalls()).toEqual([{ action: "confirmed", revision: 0 }]);
    expect(reload).toHaveBeenCalled();
    expect(container.textContent).toMatch(/link kept/i);
    expect(container.textContent).toContain("0 to review · 1 kept");
    await click(/^undo$/i);
    expect(respondCalls()[1]).toMatchObject({ action: "rolled_back", revision: 1, target_revision: 0 });
    expect(container.textContent).toContain("1 to review · 0 kept");
  });

  it("requires a reason before marking a link as not real", async () => {
    await render();
    await click(/think about this link/i);
    await click(/show me the passages/i);
    await click(/not a real link/i);
    const confirmReject = button(/confirm: not a real link/i)!;
    expect(confirmReject.disabled).toBe(true);
    await click(/only share a word/i);
    expect(button(/confirm: not a real link/i)!.disabled).toBe(false);
    await click(/confirm: not a real link/i);
    expect(respondCalls()).toEqual([{ action: "rejected", revision: 0, reason: "They only share a word" }]);
  });

  it("relabels with the learner's own note", async () => {
    await render();
    await click(/think about this link/i);
    await click(/show me the passages/i);
    await click(/different relationship/i);
    const field = container.querySelector("input[maxlength='160']") as HTMLInputElement;
    await type(field, "builds on");
    await click(/keep link with this note/i);
    expect(respondCalls()).toEqual([{ action: "relabeled", revision: 0, label: "builds on" }]);
  });

  it("fails closed when a passage or its location is missing", async () => {
    currentPreview = preview({ target_quote: " " });
    await render();
    expect(container.querySelector('[role="alert"]')?.textContent).toMatch(/cannot be reviewed/i);
    expect(button(/think about this link/i)!.disabled).toBe(true);
    currentPreview = preview({ source_locator: undefined });
    await act(async () => root.unmount());
    root = createRoot(container);
    await render();
    expect(button(/think about this link/i)!.disabled).toBe(true);
  });

  it("skip moves to the next suggestion", async () => {
    links = [link, { ...link, id: "link-2", target_document_title: "Week 4 notes: planning revision", suggested_at: "2026-10-02T00:00:00Z" }];
    await render();
    expect(container.textContent).toContain("Suggestion 1 of 2");
    expect(container.textContent).toContain("Week 3 notes: how memory works");
    await click(/skip for now/i);
    expect(container.textContent).toContain("Week 4 notes: planning revision");
  });

  it("shows an empty state and kept links", async () => {
    links = [{ ...link, status: "confirmed", revision: 1 }];
    await render();
    expect(container.textContent).toMatch(/nothing left to review/i);
    await click(/see kept links/i);
    expect(container.textContent).toContain("↔ Week 3 notes: how memory works");
    expect(container.textContent).toContain("Shared idea: Retrieval Practice");
  });
});
