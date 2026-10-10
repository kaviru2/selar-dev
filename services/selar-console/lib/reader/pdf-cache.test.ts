import { describe, expect, it } from "vitest";
import {
  MAX_AGE_MS,
  MAX_ENTRIES,
  PDF_CACHE_NAME,
  dropOtherUsersPdfs,
  forgetDocumentPdf,
  pdfCacheKey,
  purgePdfCache,
  readCachedPdf,
  sha256Hex,
  storeCachedPdf,
  type CacheStorageLike,
} from "./pdf-cache";

// Minimal in-memory Cache Storage (jsdom has none).
class MemoryCache {
  entries = new Map<string, Response>();
  private path(request: RequestInfo | URL) {
    const url = typeof request === "string" ? request : request instanceof URL ? request.href : request.url;
    return new URL(url, "http://cache.invalid").pathname;
  }
  async match(request: RequestInfo | URL) {
    return this.entries.get(this.path(request))?.clone();
  }
  async put(request: RequestInfo | URL, response: Response) {
    this.entries.set(this.path(request), response);
  }
  async delete(request: RequestInfo | URL) {
    return this.entries.delete(this.path(request));
  }
  async keys() {
    return [...this.entries.keys()].map((path) => new Request(`http://cache.invalid${path}`));
  }
}
class MemoryStorage {
  caches = new Map<string, MemoryCache>();
  async open(name: string) {
    if (!this.caches.has(name)) this.caches.set(name, new MemoryCache());
    return this.caches.get(name)! as unknown as Cache;
  }
  async delete(name: string) {
    return this.caches.delete(name);
  }
}

// Compare as plain arrays: jsdom and Node have different Uint8Array realms.
const bytesOf = (value: Uint8Array | null) => (value ? Array.from(value) : value);
const pdf = (text: string) => new TextEncoder().encode(`%PDF-1.7 ${text}`);
async function keyFor(userId: string, docId: string, bytes: Uint8Array) {
  return { userId, docId, contentHash: await sha256Hex(bytes) };
}
const fresh = () => new MemoryStorage() as unknown as CacheStorageLike & MemoryStorage;

describe("pdf cache", () => {
  it("returns the stored bytes on the next open for the same user, document and hash", async () => {
    const store = fresh();
    const bytes = pdf("alpha");
    const key = await keyFor("user-a", "doc-1", bytes);
    expect(await readCachedPdf(key, store)).toBeNull();
    expect(await storeCachedPdf(key, bytes, store)).toBe(true);
    expect(bytesOf(await readCachedPdf(key, store))).toEqual(Array.from(bytes));
  });

  it("never serves one user's PDF to another user", async () => {
    const store = fresh();
    const bytes = pdf("private");
    const key = await keyFor("user-a", "doc-1", bytes);
    await storeCachedPdf(key, bytes, store);
    expect(await readCachedPdf({ ...key, userId: "user-b" }, store)).toBeNull();
  });

  it("refuses bytes whose SHA-256 does not match the document hash", async () => {
    const store = fresh();
    const key = await keyFor("user-a", "doc-1", pdf("expected"));
    expect(await storeCachedPdf(key, pdf("something else"), store)).toBe(false);
    expect(await readCachedPdf(key, store)).toBeNull();
  });

  it("misses when the document changed (new content hash) and drops the old version", async () => {
    const store = fresh();
    const v1 = pdf("v1");
    const v2 = pdf("v2");
    const k1 = await keyFor("user-a", "doc-1", v1);
    const k2 = await keyFor("user-a", "doc-1", v2);
    await storeCachedPdf(k1, v1, store);
    expect(await readCachedPdf(k2, store)).toBeNull();
    await storeCachedPdf(k2, v2, store);
    expect(await readCachedPdf(k1, store)).toBeNull();
    expect(bytesOf(await readCachedPdf(k2, store))).toEqual(Array.from(v2));
  });

  it("does not cache without a user id or a sha256 content hash", async () => {
    const store = fresh();
    const bytes = pdf("x");
    const key = await keyFor("user-a", "doc-1", bytes);
    expect(pdfCacheKey({ ...key, userId: "" })).toBeNull();
    expect(pdfCacheKey({ ...key, contentHash: "" })).toBeNull();
    expect(pdfCacheKey({ ...key, docId: "../other" })).toBeNull();
    expect(await storeCachedPdf({ ...key, userId: "" }, bytes, store)).toBe(false);
  });

  it("purges everything (sign-out, account deletion)", async () => {
    const store = fresh();
    const bytes = pdf("a");
    const key = await keyFor("user-a", "doc-1", bytes);
    await storeCachedPdf(key, bytes, store);
    await purgePdfCache(store);
    expect(store.caches.has(PDF_CACHE_NAME)).toBe(false);
    expect(await readCachedPdf(key, store)).toBeNull();
  });

  it("forgets one deleted document and keeps the others", async () => {
    const store = fresh();
    const a = pdf("a");
    const b = pdf("b");
    const ka = await keyFor("user-a", "doc-a", a);
    const kb = await keyFor("user-a", "doc-b", b);
    await storeCachedPdf(ka, a, store);
    await storeCachedPdf(kb, b, store);
    await forgetDocumentPdf("doc-a", store);
    expect(await readCachedPdf(ka, store)).toBeNull();
    expect(bytesOf(await readCachedPdf(kb, store))).toEqual(Array.from(b));
  });

  it("drops another account's PDFs when a different user opens the reader", async () => {
    const store = fresh();
    const a = pdf("a");
    const b = pdf("b");
    const ka = await keyFor("user-a", "doc-a", a);
    const kb = await keyFor("user-b", "doc-b", b);
    await storeCachedPdf(ka, a, store);
    // Storing for user-b also evicts user-a's leftovers.
    await storeCachedPdf(kb, b, store);
    expect(await readCachedPdf(ka, store)).toBeNull();
    expect(bytesOf(await readCachedPdf(kb, store))).toEqual(Array.from(b));
    // And opening the reader as user-a drops user-b's entries.
    await dropOtherUsersPdfs("user-a", store);
    expect(await readCachedPdf(kb, store)).toBeNull();
  });

  it("expires entries older than the max age", async () => {
    const store = fresh();
    const bytes = pdf("old");
    const key = await keyFor("user-a", "doc-1", bytes);
    await storeCachedPdf(key, bytes, store, 1_000);
    expect(await readCachedPdf(key, store, 1_000 + MAX_AGE_MS + 1)).toBeNull();
  });

  it("keeps at most MAX_ENTRIES documents, evicting the oldest", async () => {
    const store = fresh();
    const keys = [];
    for (let i = 0; i <= MAX_ENTRIES; i++) {
      const bytes = pdf(`doc ${i}`);
      const key = await keyFor("user-a", `doc-${i}`, bytes);
      keys.push(key);
      await storeCachedPdf(key, bytes, store, 1_000 + i);
    }
    expect(await readCachedPdf(keys[0], store, 2_000)).toBeNull();
    expect(await readCachedPdf(keys[MAX_ENTRIES], store, 2_000)).not.toBeNull();
    expect((await (await store.open(PDF_CACHE_NAME)).keys()).length).toBe(MAX_ENTRIES);
  });

  it("is a no-op when Cache Storage is unavailable", async () => {
    const bytes = pdf("x");
    const key = await keyFor("user-a", "doc-1", bytes);
    expect(await storeCachedPdf(key, bytes, null)).toBe(false);
    expect(await readCachedPdf(key, null)).toBeNull();
    await expect(purgePdfCache(null)).resolves.toBeUndefined();
  });
});
