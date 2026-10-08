import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AddContentButton } from "./AddContentButton";
import { UploadButton } from "./UploadButton";
import SettingsPage from "../app/(app)/settings/page";
import OnboardingPage from "../app/(app)/onboarding/page";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }), useSearchParams: () => ({ get: () => null }) }));
vi.mock("@/lib/context", () => ({ useSelar: () => ({ user: null, theme: "paper", density: "balanced", setTheme: vi.fn(), setDensity: vi.fn() }) }));

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute("open", ""); };
  HTMLDialogElement.prototype.close = function () { this.removeAttribute("open"); };
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.unstubAllGlobals(); });
const render = async (node: React.ReactNode) => { await act(async () => root.render(node)); };
const disclosure = (text: string) => {
  expect(text).toMatch(/source text.*SELAR server/i);
  expect(text).toMatch(/Google Gemini.*embeddings.*AI link generation/i);
};

describe("processing disclosure in real entry points", () => {
  it("is visible before web, text, and PDF submissions in the add-content dialog", async () => {
    await render(<AddContentButton />);
    await act(async () => { (container.querySelector("button.btn.primary") as HTMLButtonElement).click(); });
    for (const label of ["From the web", "Paste text", "Upload PDF"]) {
      await act(async () => { (Array.from(container.querySelectorAll('[role="tab"]')).find((tab) => tab.textContent?.includes(label)) as HTMLButtonElement).click(); });
      disclosure(container.querySelector("dialog[open]")?.textContent || "");
    }
  });
  it("discloses processing next to the direct upload control before file choice", async () => {
    await render(<UploadButton />);
    disclosure(container.textContent || "");
    expect(container.querySelector('input[type="file"]')?.getAttribute("aria-label")).toBeTruthy();
    expect(container.querySelector('input[type="file"]')?.getAttribute("tabindex")).not.toBe("-1");
    expect((container.querySelector('input[type="file"]') as HTMLElement).style.display).not.toBe("none");
  });
  it("discloses processing on onboarding and settings without local-only claims", () => {
    for (const html of [renderToStaticMarkup(<OnboardingPage />), renderToStaticMarkup(<SettingsPage />)]) {
      disclosure(html);
      expect(html).not.toMatch(/stored locally; no third-party analytics/i);
    }
  });
  it("does not promise preservation or account privacy guarantees in entry copy", async () => {
    await render(<AddContentButton />);
    await act(async () => { (container.querySelector("button.btn.primary") as HTMLButtonElement).click(); });
    expect(container.querySelector("dialog")?.textContent).not.toMatch(/processing history are preserved/i);
    expect(renderToStaticMarkup(<OnboardingPage />)).not.toMatch(/library is private to your account/i);
  });
  it("offers a named keyboard-operable PDF chooser, rather than a hidden file input", async () => {
    await render(<AddContentButton />);
    await act(async () => { (container.querySelector("button.btn.primary") as HTMLButtonElement).click(); });
    await act(async () => { (Array.from(container.querySelectorAll('[role="tab"]')).find((tab) => tab.textContent?.includes("Upload PDF")) as HTMLButtonElement).click(); });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    expect(input.getAttribute("aria-label")).toBeTruthy();
    expect(input.tabIndex).not.toBe(-1);
    expect(input.style.display).not.toBe("none");
    const zone = container.querySelector('.pdf-dropzone') as HTMLElement;
    expect(zone.getAttribute("aria-label")).toBeTruthy();
    expect(zone.tabIndex).toBe(0);
    const chooser = vi.spyOn(input, "click").mockImplementation(() => {});
    await act(async () => zone.dispatchEvent(new KeyboardEvent("keydown", { key: " ", bubbles: true, cancelable: true })));
    expect(chooser).toHaveBeenCalledOnce();
  });
  it("announces direct-upload server errors and resets the chooser for retry", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, json: async () => ({ error: "Upload rejected" }) }));
    await render(<UploadButton />);
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, "files", { configurable: true, value: [new File(["synthetic"], "synthetic.pdf", { type: "application/pdf" })] });
    await act(async () => input.dispatchEvent(new Event("change", { bubbles: true })));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("Upload rejected");
    expect(container.querySelector('[role="status"]')).toBeTruthy();
    expect(input.value).toBe("");
  });
  it("uploads large PDFs from the add-content dialog straight to storage, then registers them", async () => {
    const calls: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (url: string, init?: RequestInit) => {
      calls.push(`${init?.method || "GET"} ${url}`);
      if (url === "/api/documents/upload-url") {
        return new Response(JSON.stringify({ mode: "direct", upload_id: "u-1", upload: { method: "PUT", url: "https://bucket.example/users/u/uploads/u-1.pdf?sig=1", headers: { "Content-Type": "application/pdf" } } }), { status: 200 });
      }
      if (url.startsWith("https://bucket.example/")) return new Response(null, { status: 200 });
      return new Response(JSON.stringify({ document: { id: "d" } }), { status: 202 });
    }));
    await render(<AddContentButton />);
    await act(async () => { (container.querySelector("button.btn.primary") as HTMLButtonElement).click(); });
    await act(async () => { (Array.from(container.querySelectorAll('[role="tab"]')).find((tab) => tab.textContent?.includes("Upload PDF")) as HTMLButtonElement).click(); });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    const large = new File([new Uint8Array(6 * 1024 * 1024)], "large-synthetic.pdf", { type: "application/pdf" });
    Object.defineProperty(input, "files", { configurable: true, value: [large] });
    await act(async () => input.dispatchEvent(new Event("change", { bubbles: true })));
    await act(async () => { (container.querySelector("dialog form") as HTMLFormElement).requestSubmit(); });
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });
    expect(calls).toEqual([
      "POST /api/documents/upload-url",
      "PUT https://bucket.example/users/u/uploads/u-1.pdf?sig=1",
      "POST /api/documents/upload-complete",
    ]);
    expect(calls.some((call) => call.includes("/api/documents/upload "))).toBe(false);
  });
});
