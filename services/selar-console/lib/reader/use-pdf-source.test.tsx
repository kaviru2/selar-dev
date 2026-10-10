import { act, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";
import { sha256Hex, storeCachedPdf, type CacheStorageLike } from "./pdf-cache";
import { pdfUrl, usePdfSource, type PdfSourceState } from "./use-pdf-source";

class MemoryCache {
  entries = new Map<string, Response>();
  private path(request: RequestInfo | URL) {
    const url = typeof request === "string" ? request : request instanceof URL ? request.href : request.url;
    return new URL(url, "http://cache.invalid").pathname;
  }
  async match(request: RequestInfo | URL) { return this.entries.get(this.path(request))?.clone(); }
  async put(request: RequestInfo | URL, response: Response) { this.entries.set(this.path(request), response); }
  async delete(request: RequestInfo | URL) { return this.entries.delete(this.path(request)); }
  async keys() { return [...this.entries.keys()].map((path) => new Request(`http://cache.invalid${path}`)); }
}
function memoryStorage(): CacheStorageLike {
  const caches = new Map<string, MemoryCache>();
  return {
    open: async (name: string) => {
      if (!caches.has(name)) caches.set(name, new MemoryCache());
      return caches.get(name)! as unknown as Cache;
    },
    delete: async (name: string) => caches.delete(name),
  };
}

const settle = () => act(async () => { for (let i = 0; i < 10; i++) await new Promise((resolve) => setTimeout(resolve, 0)); });

async function render(docId: string, userId: string, contentHash: string, store: CacheStorageLike) {
  const seen: { current: PdfSourceState | null } = { current: null };
  function Probe() {
    const state = usePdfSource(docId, { userId, contentHash }, store);
    useEffect(() => { seen.current = state; });
    return null;
  }
  // React's act() needs this flag outside a test renderer.
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  const root = createRoot(document.createElement("div"));
  await act(async () => root.render(<Probe />));
  await settle();
  return { seen, unmount: () => act(async () => root.unmount()) };
}

describe("usePdfSource", () => {
  it("loads from the network with range loading on a miss, then caches the full file", async () => {
    const store = memoryStorage();
    const bytes = new TextEncoder().encode("%PDF-1.7 network copy");
    const hash = await sha256Hex(bytes);
    const first = await render("doc-1", "user-a", hash, store);
    expect(first.seen.current?.source).toEqual({ url: pdfUrl("doc-1") });
    expect(first.seen.current?.fromCache).toBe(false);

    let downloaded!: () => void;
    const done = new Promise<{ length: number }>((resolve) => { downloaded = () => resolve({ length: bytes.length }); });
    let gotData = false;
    first.seen.current!.rememberPdf({ getDownloadInfo: () => done, getData: async () => { gotData = true; return bytes; } });
    await settle();
    // Must not pull the whole file before the background download finished.
    expect(gotData).toBe(false);
    downloaded();
    await settle();
    expect(gotData).toBe(true);
    await first.unmount();

    const second = await render("doc-1", "user-a", hash, store);
    const source = second.seen.current?.source as { data: Uint8Array };
    expect(second.seen.current?.fromCache).toBe(true);
    expect(Array.from(source.data)).toEqual(Array.from(bytes));
    await second.unmount();
  });

  it("does not reuse one account's cached PDF for another account", async () => {
    const store = memoryStorage();
    const bytes = new TextEncoder().encode("%PDF-1.7 private");
    const hash = await sha256Hex(bytes);
    await storeCachedPdf({ userId: "user-a", docId: "doc-1", contentHash: hash }, bytes, store);
    const other = await render("doc-1", "user-b", hash, store);
    expect(other.seen.current?.fromCache).toBe(false);
    expect(other.seen.current?.source).toEqual({ url: pdfUrl("doc-1") });
    await other.unmount();
    // user-b opening the reader also removed user-a's copy from this device.
    const owner = await render("doc-1", "user-a", hash, store);
    expect(owner.seen.current?.fromCache).toBe(false);
    await owner.unmount();
  });
});
