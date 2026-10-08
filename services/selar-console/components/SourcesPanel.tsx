"use client";

import { useCallback, useEffect, useState } from "react";
import { clientFetch, type ContentSource } from "@/lib/api";
import { Icon } from "@/components/ui/Icon";

export function SourcesPanel() {
  const [sources, setSources] = useState<ContentSource[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [bookmarkletCopied, setBookmarkletCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      setSources(await clientFetch<ContentSource[]>("/api/sources"));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to load sources");
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    clientFetch<ContentSource[]>("/api/sources")
      .then((items) => { if (!cancelled) setSources(items); })
      .catch((cause) => { if (!cancelled) setError(cause instanceof Error ? cause.message : "Unable to load sources"); });
    return () => { cancelled = true; };
  }, []);

  async function refresh(source: ContentSource) {
    setBusy(source.id);
    setError("");
    try {
      await clientFetch(`/api/sources/${source.id}/refresh`, { method: "POST" });
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to refresh source");
    } finally {
      setBusy("");
    }
  }

  async function archive(source: ContentSource) {
    if (!confirm("Archive this source? Existing document snapshots remain in the research library.")) return;
    setBusy(source.id);
    try {
      await clientFetch(`/api/sources/${source.id}`, { method: "DELETE" });
      await load();
    } finally {
      setBusy("");
    }
  }

  async function copyBookmarklet() {
    const destination = `${window.location.origin}/library?addUrl=`;
    const code = `javascript:location.href=${JSON.stringify(destination)}+encodeURIComponent(location.href)`;
    await navigator.clipboard.writeText(code);
    setBookmarkletCopied(true);
    window.setTimeout(() => setBookmarkletCopied(false), 1800);
  }

  return (
    <section className="sources-panel">
      <div className="sources-head">
        <div><h2>Where your readings came from</h2><p>Each source, when it was last fetched, and whether processing worked.</p></div>
        <button className="btn" onClick={copyBookmarklet}>{bookmarkletCopied ? "Copied" : "Copy Save bookmarklet"}</button>
        <span className="sources-count" aria-label={`${sources.length} sources`}>{sources.length}</span>
      </div>
      {error && <div className="form-error">{error}</div>}
      {sources.length === 0 ? <div className="sources-empty">Sources show up here once you add an article, some notes, or a PDF.</div> : (
        <div className="source-list">
          {sources.map((source) => (
            <div className="source-row" key={source.id}>
              <Icon name={source.kind === "text" ? "note" : "doc"} size={13} />
              <div className="source-main">
                <strong>{source.title || source.canonical_uri || "Untitled source"}</strong>
                <span>{source.kind} · {source.last_fetched_at ? `fetched ${new Date(source.last_fetched_at).toLocaleString()}` : "queued"}</span>
                {source.last_error && <em>{source.last_error}</em>}
              </div>
              <span className={`source-status ${source.status}`}>{source.status}</span>
              {source.kind === "web" && <button className="btn" disabled={busy === source.id} onClick={() => refresh(source)}>Refresh</button>}
              <button className="icon-button" aria-label="Archive source" disabled={busy === source.id} onClick={() => archive(source)}><Icon name="trash" size={12} /></button>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
