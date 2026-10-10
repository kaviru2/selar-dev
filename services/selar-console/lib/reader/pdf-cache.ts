// pdf-cache.ts — per-user, on-device cache of PDF bytes for the Reader.
//
// Why: the API answers /api/documents/{id}/pdf with a 302 to a freshly
// presigned bucket URL, so the browser HTTP cache never hits and every open
// re-downloaded the whole file. Here the verified bytes are kept in the
// origin's Cache Storage, keyed by user + document + content hash.
//
// Privacy rules:
// - Keys include the user id, and lookups only ever use the signed-in user's id.
// - Bytes are stored only if their SHA-256 equals the document's content_hash,
//   so a key can never hold another document's bytes.
// - Everything is purged on sign-out, account deletion and before a new sign-in
//   (purgePdfCache), and per document on delete (forgetDocumentPdf).
// - Nothing goes through a shared or public cache: this is browser-local storage.

export const PDF_CACHE_NAME = "selar-pdf-v1";
/** Keep at most this many PDFs… */
export const MAX_ENTRIES = 12;
/** …and at most this many bytes in total. */
export const MAX_TOTAL_BYTES = 300 * 1024 * 1024;
/** Entries older than this are dropped on read. */
export const MAX_AGE_MS = 14 * 24 * 60 * 60 * 1000;

const KEY_PREFIX = "/__selar-pdf-cache/";
const STORED_AT = "X-Selar-Stored-At";
const SAFE = /^[A-Za-z0-9-]{1,128}$/;
const HASH = /^[a-f0-9]{64}$/;

export type CacheStorageLike = Pick<CacheStorage, "open" | "delete">;

export interface PdfCacheKey {
  userId: string;
  docId: string;
  contentHash: string;
}

function storage(override?: CacheStorageLike | null): CacheStorageLike | null {
  if (override !== undefined) return override;
  return typeof caches === "undefined" ? null : caches;
}

/** Returns null when the key is not cacheable (missing/odd ids, non-sha256 hash). */
export function pdfCacheKey({ userId, docId, contentHash }: PdfCacheKey): string | null {
  const hash = contentHash.toLowerCase();
  if (!SAFE.test(userId) || !SAFE.test(docId) || !HASH.test(hash)) return null;
  return `${KEY_PREFIX}${userId}/${docId}/${hash}`;
}

function keyPath(request: Request | string): string {
  const url = typeof request === "string" ? request : request.url;
  try {
    return new URL(url, "http://cache.invalid").pathname;
  } catch {
    return url;
  }
}

export async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes as BufferSource);
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, "0")).join("");
}

/** Cached bytes for this user's document version, or null. Never throws. */
export async function readCachedPdf(key: PdfCacheKey, cacheStorage?: CacheStorageLike | null, now = Date.now()): Promise<Uint8Array | null> {
  const path = pdfCacheKey(key);
  const store = storage(cacheStorage);
  if (!path || !store) return null;
  try {
    const cache = await store.open(PDF_CACHE_NAME);
    const hit = await cache.match(path);
    if (!hit) return null;
    const storedAt = Number(hit.headers.get(STORED_AT) || 0);
    if (!storedAt || now - storedAt > MAX_AGE_MS) {
      await cache.delete(path);
      return null;
    }
    return new Uint8Array(await hit.arrayBuffer());
  } catch {
    return null;
  }
}

/**
 * Stores verified bytes. Drops older versions of the same document, then
 * evicts the oldest entries beyond MAX_ENTRIES / MAX_TOTAL_BYTES.
 * Returns true when stored. Never throws.
 */
export async function storeCachedPdf(key: PdfCacheKey, bytes: Uint8Array, cacheStorage?: CacheStorageLike | null, now = Date.now()): Promise<boolean> {
  const path = pdfCacheKey(key);
  const store = storage(cacheStorage);
  if (!path || !store || bytes.byteLength === 0 || bytes.byteLength > MAX_TOTAL_BYTES) return false;
  try {
    if ((await sha256Hex(bytes)) !== key.contentHash.toLowerCase()) return false;
    const cache = await store.open(PDF_CACHE_NAME);
    const docPrefix = `${KEY_PREFIX}${key.userId}/${key.docId}/`;
    const entries: Array<{ path: string; at: number; size: number }> = [];
    for (const request of await cache.keys()) {
      const existing = keyPath(request);
      if (existing.startsWith(docPrefix) || !existing.startsWith(`${KEY_PREFIX}${key.userId}/`)) {
        // Older versions of this document, or (defensively) another user's entry.
        await cache.delete(request);
        continue;
      }
      const response = await cache.match(request);
      entries.push({ path: existing, at: Number(response?.headers.get(STORED_AT) || 0), size: Number(response?.headers.get("Content-Length") || 0) });
    }
    entries.sort((a, b) => a.at - b.at);
    let total = entries.reduce((sum, entry) => sum + entry.size, 0) + bytes.byteLength;
    let count = entries.length + 1;
    for (const entry of entries) {
      if (count <= MAX_ENTRIES && total <= MAX_TOTAL_BYTES) break;
      await cache.delete(entry.path);
      total -= entry.size;
      count -= 1;
    }
    await cache.put(path, new Response(bytes as BodyInit, {
      headers: { "Content-Type": "application/pdf", "Content-Length": String(bytes.byteLength), [STORED_AT]: String(now) },
    }));
    return true;
  } catch {
    return false;
  }
}

/** Removes every cached version of one document (after it is deleted). */
export async function forgetDocumentPdf(docId: string, cacheStorage?: CacheStorageLike | null): Promise<void> {
  const store = storage(cacheStorage);
  if (!store || !SAFE.test(docId)) return;
  try {
    const cache = await store.open(PDF_CACHE_NAME);
    for (const request of await cache.keys()) {
      if (keyPath(request).split("/")[3] === docId) await cache.delete(request);
    }
  } catch {
    // Best effort; entries are still bound to the owner and expire.
  }
}

/** Removes cached PDFs that belong to anyone but `userId`. */
export async function dropOtherUsersPdfs(userId: string, cacheStorage?: CacheStorageLike | null): Promise<void> {
  const store = storage(cacheStorage);
  if (!store || !SAFE.test(userId)) return;
  try {
    const cache = await store.open(PDF_CACHE_NAME);
    for (const request of await cache.keys()) {
      if (!keyPath(request).startsWith(`${KEY_PREFIX}${userId}/`)) await cache.delete(request);
    }
  } catch {
    // Best effort.
  }
}

/** Drops every cached PDF on this device (sign-out, account deletion, new sign-in). */
export async function purgePdfCache(cacheStorage?: CacheStorageLike | null): Promise<void> {
  const store = storage(cacheStorage);
  if (!store) return;
  try {
    await store.delete(PDF_CACHE_NAME);
  } catch {
    // Storage unavailable (private mode etc.): nothing was cached.
  }
}
