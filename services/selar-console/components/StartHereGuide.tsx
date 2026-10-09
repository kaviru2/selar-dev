"use client";

import Link from "next/link";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useSelar } from "@/lib/context";
import type { Document } from "@/lib/api";
import { RetryIngestionButton } from "./RetryIngestionButton";

function subscribe(listener: () => void) {
  window.addEventListener("storage", listener);
  window.addEventListener("selar-guide-change", listener);
  return () => {
    window.removeEventListener("storage", listener);
    window.removeEventListener("selar-guide-change", listener);
  };
}
function read(key: string) {
  try { return window.localStorage.getItem(key); } catch { return null; }
}
function save(key: string, value: string) {
  try { window.localStorage.setItem(key, value); } catch { /* Storage is optional. */ }
  window.dispatchEvent(new Event("selar-guide-change"));
}

export function ReadingOpenLink({ id, title }: { id: string; title: string }) {
  const { user } = useSelar();
  return <Link className="lib-title-link" href={`/reader?docId=${encodeURIComponent(id)}`}
    onClick={() => { if (user) save(`selar:start-here:v1:${user.id}`, "complete"); }}>{title}</Link>;
}

export function StartHereGuide({ documents }: { documents: Document[] }) {
  const ready = documents.find((d) => d.status === "ready");
  const pending = documents.find((d) => d.status === "uploaded" || d.status === "processing");
  const failed = documents.find((d) => d.status === "failed");
  const { user } = useSelar();
  const key = `selar:start-here:v1:${user?.id ?? "anonymous"}`;
  const saved = useSyncExternalStore(subscribe, () => read(key), () => null);
  const [expanded, setExpanded] = useState<boolean | null>(null);
  const summary = useRef<HTMLElement>(null);
  useEffect(() => {
    // Hydration initially sees the server snapshot, not stored dismissal.
    // Re-read before enrolling so returning users never lose that choice.
    if (user && !ready && !saved && !read(key)) save(key, "started");
  }, [key, ready, saved, user]);
  const open = expanded ?? (saved === "dismissed" || saved === "complete" ? false : saved === "started" || !ready);
  function finish(value: "dismissed" | "complete") {
    save(key, value);
    setExpanded(false);
    if (value === "dismissed") summary.current?.focus();
  }
  return <details id="start-here" open={open} className="ui-card" style={{ padding: 18, marginBottom: 20 }}>
    <summary ref={summary} onClick={(event) => { event.preventDefault(); setExpanded(!open); }} style={{ cursor: "pointer", fontWeight: 600 }}>Start here · your first reading</summary>
    <div style={{ display: "grid", gap: 12, paddingTop: 12 }}>
      {ready ? <><p>Ready to read: {ready.title}. Processing is complete; it does not measure what you have learned.</p><Link onClick={() => finish("complete")} href={`/reader?docId=${encodeURIComponent(ready.id)}`}>Open {ready.title}</Link></>
        : pending ? <p role="status">{pending.status === "uploaded" || pending.ingestion_status === "queued" ? "Queued" : "Processing"}: {pending.title}. You can leave and come back; the library updates while you’re here.</p>
        : failed ? <div><p>{failed.title} couldn’t be processed. Retry, or choose another reading.</p><RetryIngestionButton documentId={failed.id} /></div>
        : <><p>Add one reading you are allowed to use. You don’t need to fill out a questionnaire.</p><a href="#library-imports">Choose an import</a></>}
      <p>Read at your own pace. With more readings, SELAR may suggest connections; inspect the source passages yourself.</p>
      <p><Link href="/settings">Optional reading preferences</Link> · <Link href="/onboarding">Getting started help</Link></p>
      <p>This guide is optional; dismissal is remembered in this browser for your account.</p>
      <button type="button" className="ui-btn ui-btn--ghost ui-btn--sm" onClick={() => finish("dismissed")}>Dismiss guide</button>
    </div>
  </details>;
}
