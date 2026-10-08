// reader-layout.ts — Reader side-panel layout: which panels are open and how
// wide they are. Pure helpers (clamping, parsing stored state, keyboard
// resizing) plus a small hook that persists the layout per device in
// localStorage. Kept separate from the reader page so it can be unit-tested.

import { useCallback, useEffect, useState } from "react";

export type PanelSide = "library" | "connections";

export interface PanelLayout {
  libraryOpen: boolean;
  connectionsOpen: boolean;
  libraryWidth: number;
  connectionsWidth: number;
}

export const PANEL_LIMITS: Record<PanelSide, { min: number; max: number; initial: number }> = {
  library: { min: 180, max: 420, initial: 240 },
  connections: { min: 280, max: 560, initial: 360 },
};

// The document column never gets narrower than this while panels are docked.
export const MIN_DOCUMENT_WIDTH = 420;
// Below this viewport width open panels float over the document (see CSS).
export const OVERLAY_BREAKPOINT = 800;
export const STORAGE_KEY = "selar.reader.layout.v1";
export const KEYBOARD_STEP = 16;

export const DEFAULT_LAYOUT: PanelLayout = {
  libraryOpen: true,
  connectionsOpen: true,
  libraryWidth: PANEL_LIMITS.library.initial,
  connectionsWidth: PANEL_LIMITS.connections.initial,
};

/** Clamp a panel width to its limits and to what the viewport can spare. */
export function clampPanelWidth(
  side: PanelSide,
  width: number,
  viewportWidth = Infinity,
  otherPanelWidth = 0,
): number {
  const { min, max } = PANEL_LIMITS[side];
  const room = viewportWidth - otherPanelWidth - MIN_DOCUMENT_WIDTH;
  const upper = Math.max(min, Math.min(max, room));
  if (!Number.isFinite(width)) return PANEL_LIMITS[side].initial;
  return Math.round(Math.min(upper, Math.max(min, width)));
}

/** First-visit layout: narrow windows start with panels closed so the document is readable. */
export function initialLayout(viewportWidth: number): PanelLayout {
  if (viewportWidth < OVERLAY_BREAKPOINT) return { ...DEFAULT_LAYOUT, libraryOpen: false, connectionsOpen: false };
  if (viewportWidth < 1200) return { ...DEFAULT_LAYOUT, connectionsOpen: false };
  return { ...DEFAULT_LAYOUT };
}

/** Parse stored layout defensively; anything malformed falls back to the given layout. */
export function parseStoredLayout(raw: string | null, fallback: PanelLayout): PanelLayout {
  if (!raw) return fallback;
  try {
    const value = JSON.parse(raw) as Partial<Record<keyof PanelLayout, unknown>>;
    if (!value || typeof value !== "object") return fallback;
    const bool = (v: unknown, d: boolean) => (typeof v === "boolean" ? v : d);
    const num = (side: PanelSide, v: unknown, d: number) =>
      typeof v === "number" ? clampPanelWidth(side, v) : d;
    return {
      libraryOpen: bool(value.libraryOpen, fallback.libraryOpen),
      connectionsOpen: bool(value.connectionsOpen, fallback.connectionsOpen),
      libraryWidth: num("library", value.libraryWidth, fallback.libraryWidth),
      connectionsWidth: num("connections", value.connectionsWidth, fallback.connectionsWidth),
    };
  } catch {
    return fallback;
  }
}

/**
 * New width after a resize-handle key press, or null if the key is not a
 * resize key. The library handle sits on the panel's right edge (ArrowRight
 * grows it); the connections handle sits on its left edge (ArrowLeft grows it).
 */
export function keyboardResize(side: PanelSide, width: number, key: string, shift = false): number | null {
  const step = shift ? KEYBOARD_STEP * 4 : KEYBOARD_STEP;
  const grow = side === "library" ? "ArrowRight" : "ArrowLeft";
  const shrink = side === "library" ? "ArrowLeft" : "ArrowRight";
  if (key === grow) return width + step;
  if (key === shrink) return width - step;
  if (key === "Home") return PANEL_LIMITS[side].min;
  if (key === "End") return PANEL_LIMITS[side].max;
  return null;
}

/** True when a keyboard shortcut should be ignored because the user is typing. */
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!target || typeof (target as HTMLElement).tagName !== "string") return false;
  const el = target as HTMLElement;
  const tag = el.tagName.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select" || el.isContentEditable === true;
}

function readViewport(): number {
  return typeof window === "undefined" ? 1440 : window.innerWidth;
}

/** Reader panel layout persisted per device. */
export function useReaderLayout() {
  // Server render and first client render use the default so hydration matches;
  // the stored (or viewport-based) layout is applied right after mount.
  const [layout, setLayout] = useState<PanelLayout>(DEFAULT_LAYOUT);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let stored: string | null = null;
    try {
      stored = window.localStorage.getItem(STORAGE_KEY);
    } catch {
      stored = null;
    }
    // eslint-disable-next-line react-hooks/set-state-in-effect -- one-time hydration from device storage
    setLayout(parseStoredLayout(stored, initialLayout(readViewport())));
    setReady(true);
  }, []);

  useEffect(() => {
    if (!ready) return;
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(layout));
    } catch {
      // Storage can be unavailable (private mode, quota); the layout still works for this visit.
    }
  }, [layout, ready]);

  const toggle = useCallback((side: PanelSide) => {
    setLayout((current) =>
      side === "library"
        ? { ...current, libraryOpen: !current.libraryOpen }
        : { ...current, connectionsOpen: !current.connectionsOpen },
    );
  }, []);

  const setOpen = useCallback((side: PanelSide, open: boolean) => {
    setLayout((current) =>
      side === "library" ? { ...current, libraryOpen: open } : { ...current, connectionsOpen: open },
    );
  }, []);

  const resize = useCallback((side: PanelSide, width: number) => {
    setLayout((current) => {
      const viewport = readViewport();
      if (side === "library") {
        const other = current.connectionsOpen ? current.connectionsWidth : 0;
        return { ...current, libraryWidth: clampPanelWidth("library", width, viewport, other) };
      }
      const other = current.libraryOpen ? current.libraryWidth : 0;
      return { ...current, connectionsWidth: clampPanelWidth("connections", width, viewport, other) };
    });
  }, []);

  const reset = useCallback((side: PanelSide) => {
    setLayout((current) =>
      side === "library"
        ? { ...current, libraryWidth: PANEL_LIMITS.library.initial }
        : { ...current, connectionsWidth: PANEL_LIMITS.connections.initial },
    );
  }, []);

  return { layout, ready, toggle, setOpen, resize, reset };
}
