// position.ts — per-document reading position and typed reader preferences.
//
// Positions live in localStorage (device-local, never sent to the API) so the
// reader reopens where the learner left off. Preferences come from the
// account's `preferences.reader` block when the settings page provides one;
// otherwise the local defaults below apply.

import type { ZoomMode } from "./navigation";

export interface ReadingPosition {
  page: number;
  /** Offset within the page, 0..1. */
  fraction: number;
  zoom: ZoomMode;
}

interface StorageLike {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

const KEY = "selar.reader.position.v1";
const MAX_DOCS = 50;

function validZoom(value: unknown): value is ZoomMode {
  return value === "fit-width" || value === "fit-page" || (typeof value === "number" && value >= 0.25 && value <= 4);
}

function validPosition(value: unknown): value is ReadingPosition & { at: number } {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return Number.isInteger(item.page) && (item.page as number) >= 1 &&
    typeof item.fraction === "number" && item.fraction >= 0 && item.fraction <= 1 &&
    validZoom(item.zoom);
}

export function createPositionStore(storage: StorageLike | null, now: () => number = () => Date.now()) {
  function readAll(): Record<string, ReadingPosition & { at: number }> {
    if (!storage) return {};
    try {
      const parsed: unknown = JSON.parse(storage.getItem(KEY) || "{}");
      return parsed && typeof parsed === "object" ? (parsed as Record<string, ReadingPosition & { at: number }>) : {};
    } catch {
      return {};
    }
  }

  return {
    load(docId: string): ReadingPosition | null {
      const entry = readAll()[docId];
      if (!validPosition(entry)) return null;
      return { page: entry.page, fraction: entry.fraction, zoom: entry.zoom };
    },
    save(docId: string, position: ReadingPosition) {
      if (!storage || !docId) return;
      const all = readAll();
      all[docId] = { ...position, at: now() };
      const ids = Object.keys(all);
      if (ids.length > MAX_DOCS) {
        ids.sort((a, b) => (all[a]?.at ?? 0) - (all[b]?.at ?? 0));
        for (const id of ids.slice(0, ids.length - MAX_DOCS)) delete all[id];
      }
      try {
        storage.setItem(KEY, JSON.stringify(all));
      } catch {
        // Quota or private mode: position memory is best-effort.
      }
    },
  };
}

export function browserPositionStore() {
  let storage: StorageLike | null = null;
  try {
    storage = typeof window !== "undefined" ? window.localStorage : null;
  } catch {
    storage = null;
  }
  return createPositionStore(storage);
}

export const HIGHLIGHT_COLORS = ["yellow", "green", "blue", "pink", "purple"] as const;
export type HighlightColor = (typeof HIGHLIGHT_COLORS)[number];

/** Reader defaults. A future `preferences.reader` block on the account overrides them. */
export interface ReaderPreferences {
  defaultZoom: ZoomMode;
  rememberPosition: boolean;
  showThumbnails: boolean;
  highlightColor: HighlightColor;
}

export const DEFAULT_READER_PREFERENCES: ReaderPreferences = {
  defaultZoom: "fit-width",
  rememberPosition: true,
  showThumbnails: false,
  highlightColor: "yellow",
};

export function readerPreferences(accountPreferences: Record<string, unknown> | undefined | null): ReaderPreferences {
  const block = accountPreferences?.reader;
  if (!block || typeof block !== "object") return { ...DEFAULT_READER_PREFERENCES };
  const raw = block as Record<string, unknown>;
  return {
    defaultZoom: validZoom(raw.defaultZoom) ? raw.defaultZoom : DEFAULT_READER_PREFERENCES.defaultZoom,
    rememberPosition: typeof raw.rememberPosition === "boolean" ? raw.rememberPosition : DEFAULT_READER_PREFERENCES.rememberPosition,
    showThumbnails: typeof raw.showThumbnails === "boolean" ? raw.showThumbnails : DEFAULT_READER_PREFERENCES.showThumbnails,
    highlightColor: (HIGHLIGHT_COLORS as readonly string[]).includes(raw.highlightColor as string)
      ? (raw.highlightColor as HighlightColor)
      : DEFAULT_READER_PREFERENCES.highlightColor,
  };
}
