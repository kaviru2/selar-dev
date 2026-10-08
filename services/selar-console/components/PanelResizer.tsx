"use client";

// PanelResizer.tsx — Drag handle between a reader side panel and the document.
// A focusable vertical separator: drag with the mouse/pen/touch, use the
// arrow keys (Shift for bigger steps) or Home/End, and double-click to reset.

import { useRef } from "react";
import { keyboardResize, PANEL_LIMITS, type PanelSide } from "@/lib/reader-layout";

interface PanelResizerProps {
  side: PanelSide;
  width: number;
  label: string;
  controls: string;
  onResize: (width: number) => void;
  onReset: () => void;
}

export function PanelResizer({ side, width, label, controls, onResize, onReset }: PanelResizerProps) {
  const drag = useRef<{ startX: number; startWidth: number } | null>(null);

  function onPointerDown(event: React.PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return;
    event.preventDefault();
    drag.current = { startX: event.clientX, startWidth: width };
    event.currentTarget.setPointerCapture?.(event.pointerId);
    document.body.classList.add("is-resizing-panel");
  }

  function onPointerMove(event: React.PointerEvent<HTMLDivElement>) {
    const state = drag.current;
    if (!state) return;
    const delta = event.clientX - state.startX;
    // Library grows to the right; Connections grows to the left.
    onResize(side === "library" ? state.startWidth + delta : state.startWidth - delta);
  }

  function endDrag(event: React.PointerEvent<HTMLDivElement>) {
    if (!drag.current) return;
    drag.current = null;
    event.currentTarget.releasePointerCapture?.(event.pointerId);
    document.body.classList.remove("is-resizing-panel");
  }

  function onKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const next = keyboardResize(side, width, event.key, event.shiftKey);
    if (next === null) return;
    event.preventDefault();
    onResize(next);
  }

  return (
    <div
      className={`panel-resizer panel-resizer-${side}`}
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-controls={controls}
      aria-valuenow={width}
      aria-valuemin={PANEL_LIMITS[side].min}
      aria-valuemax={PANEL_LIMITS[side].max}
      tabIndex={0}
      title={`${label} (drag, or use arrow keys; double-click to reset)`}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onDoubleClick={onReset}
      onKeyDown={onKeyDown}
    />
  );
}
