import { act } from "react";
import { createRoot, hydrateRoot, type Root } from "react-dom/client";
import { renderToString } from "react-dom/server";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { StartHereGuide } from "./StartHereGuide";
import type { Document } from "@/lib/api";
vi.mock("@/lib/context", () => ({ useSelar: () => ({ user: { id: "learner-a" } }) }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let host: HTMLDivElement;
let root: Root;
beforeEach(() => { window.localStorage.clear(); host = document.createElement("div"); document.body.append(host); root = createRoot(host); });
afterEach(() => { act(() => root.unmount()); host.remove(); vi.restoreAllMocks(); });
const render = (documents: Document[] = []) => act(async () => root.render(<StartHereGuide documents={documents} />));
export const reading = (status: Document["status"], extra = {}): Document => ({ id: "reading-1", user_id: "learner-a", title: "Course notes", status, authors: "", year: 0, page_count: 0, progress: 0, added_at: "", processed_at: null, source_type: "text", ...extra });
it.each([
  ["uploaded", undefined, "Queued"],
  ["processing", "queued", "Queued"],
  ["processing", "leased", "Processing"],
  ["failed", "failed", "couldn’t be processed"],
] as const)("reflects %s / %s without opening unavailable material", async (status, ingestion_status, label) => {
  await render([reading(status, { ingestion_status })]);
  expect(host.textContent).toContain(label);
  expect(host.querySelector('a[href^="/reader"]')).toBeNull();
  expect(host.textContent?.includes("Retry")).toBe(status === "failed");
});
it("offers the actual ready reading, not an invented learning result", async () => {
  await render([reading("ready")]);
  expect(host.querySelector('a[href="/reader?docId=reading-1"]')?.textContent).toContain("Open");
  expect(host.querySelector("details")?.open).toBe(false);
  expect(host.textContent).not.toMatch(/mastered|quiz|streak/i);
});
it("keeps the first-reading guide open through processing and ready, then remembers dismissal", async () => {
  await render();
  await render([reading("processing")]);
  await render([reading("ready")]);
  expect(host.querySelector("details")?.open).toBe(true);
  const dismiss = [...host.querySelectorAll("button")].find((b) => b.textContent === "Dismiss guide")!;
  expect(dismiss).toBeDefined();
  await act(async () => dismiss.click());
  expect(host.querySelector("details")?.open).toBe(false);
  expect(document.activeElement).toBe(host.querySelector("summary"));
  act(() => root.unmount()); root = createRoot(host);
  await render();
  expect(host.querySelector("details")?.open).toBe(false);
});
it("opening a ready reading completes only the guide", async () => {
  await render(); await render([reading("ready")]);
  const link = host.querySelector<HTMLAnchorElement>('a[href^="/reader"]')!;
  link.addEventListener("click", (event) => event.preventDefault());
  await act(async () => link.click());
  expect(host.querySelector("details")?.open).toBe(false);
  expect(window.localStorage.getItem("selar:start-here:v1:learner-a")).toBe("complete");
});
it("still works when browser storage is unavailable", async () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("blocked"); });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("blocked"); });
  await render();
  const dismiss = [...host.querySelectorAll("button")].find((b) => b.textContent === "Dismiss guide")!;
  expect(dismiss).toBeDefined();
  await act(async () => dismiss.click());
  expect(host.querySelector("details")?.open).toBe(false);
});
it.each(["dismissed", "complete"])("preserves %s when hydrating server-rendered HTML", async (saved) => {
  act(() => root.unmount());
  window.localStorage.setItem("selar:start-here:v1:learner-a", saved);
  host.innerHTML = renderToString(<StartHereGuide documents={[]} />);
  await act(async () => { root = hydrateRoot(host, <StartHereGuide documents={[]} />); });
  expect(window.localStorage.getItem("selar:start-here:v1:learner-a")).toBe(saved);
  expect(host.querySelector("details")?.open).toBe(false);
});
it("opens a lightweight first-reading guide without a questionnaire", async () => {
  await render();
  expect(host.querySelector("details")?.open).toBe(true);
  expect(host.textContent).toContain("Add one reading");
  expect(host.querySelector('a[href="#library-imports"]')).not.toBeNull();
  expect(host.querySelector("input, select")).toBeNull();
});
