// use-pdf-source.ts — what the Reader hands to pdf.js for one document.
//
// Cache hit: the verified bytes from the per-user on-device cache, so the
// document opens with no network transfer.
// Cache miss: the API URL with incremental loading (range requests +
// streaming), so the first page renders before the whole file has arrived. Once
// pdf.js has the complete file, rememberPdf() stores it for the next open.

import { useCallback, useEffect, useState } from "react";
import { dropOtherUsersPdfs, readCachedPdf, storeCachedPdf, type CacheStorageLike, type PdfCacheKey } from "./pdf-cache";

/** pdf.js loading options: fetch ranges on demand instead of one big blob. */
export const PDF_RANGE_OPTIONS = {
  disableRange: false,
  // Range-only: once the first response shows the server supports ranges,
  // pdf.js drops the whole-file stream and asks only for what pages need.
  // A parallel whole-file stream would hold the browser's cache lock for the
  // URL and make the first page wait for the full download.
  disableStream: true,
  // Keep filling gaps in the background after the visible pages, so the
  // complete file arrives and can be cached for the next open.
  disableAutoFetch: false,
  rangeChunkSize: 1024 * 1024,
} as const;

export type PdfSource = { url: string } | { data: Uint8Array };

export interface PdfSourceState {
  /** null while the cache lookup is in flight (a few ms). */
  source: PdfSource | null;
  fromCache: boolean;
  /** Call with the loaded document; stores its bytes when they came from the network. */
  rememberPdf: (pdf: LoadedPdf) => void;
}

/** The parts of PDFDocumentProxy used here. */
export interface LoadedPdf {
  getData: () => Promise<Uint8Array>;
  getDownloadInfo: () => Promise<{ length: number }>;
}

export interface PdfOwner {
  userId?: string;
  contentHash?: string;
}

export function pdfUrl(docId: string): string {
  return `/api/documents/${encodeURIComponent(docId)}/pdf`;
}

export function usePdfSource(docId: string, owner: PdfOwner = {}, cacheStorage?: CacheStorageLike | null): PdfSourceState {
  const userId = owner.userId || "";
  const contentHash = owner.contentHash || "";
  const [state, setState] = useState<{ key: string; source: PdfSource; fromCache: boolean } | null>(null);
  const current = `${userId}/${docId}/${contentHash}`;

  useEffect(() => {
    let cancelled = false;
    const key: PdfCacheKey = { userId, docId, contentHash };
    // A different account on this browser (e.g. after an expired session) must
    // not leave its PDFs behind for this one.
    dropOtherUsersPdfs(userId, cacheStorage)
      .then(() => readCachedPdf(key, cacheStorage))
      .then((bytes) => {
        if (cancelled) return;
        setState(bytes ? { key: current, source: { data: bytes }, fromCache: true } : { key: current, source: { url: pdfUrl(docId) }, fromCache: false });
      });
    return () => { cancelled = true; };
  }, [current, userId, docId, contentHash, cacheStorage]);

  const fresh = state?.key === current ? state : null;
  const fromCache = Boolean(fresh?.fromCache);
  const rememberPdf = useCallback((pdf: LoadedPdf) => {
    if (fromCache || !userId || !contentHash) return;
    // getDownloadInfo() resolves only after the background stream has
    // delivered the whole file. Calling getData() earlier would make pdf.js
    // request every missing range at once and compete with the visible pages.
    pdf.getDownloadInfo()
      .then(() => pdf.getData())
      .then((bytes) => storeCachedPdf({ userId, docId, contentHash }, bytes, cacheStorage))
      .catch(() => {});
  }, [fromCache, userId, docId, contentHash, cacheStorage]);

  return { source: fresh?.source ?? null, fromCache, rememberPdf };
}
