"use client";
import { useEffect, useRef, useState } from "react";
import type { Annotation } from "@/lib/api";

export const annotationColors: Annotation["color"][] = ["yellow", "wheat", "coral", "sage"];
interface EditorProps { title: string; quote?: string; color: Annotation["color"]; comment: string; onSave: (color: Annotation["color"], comment: string) => Promise<void>; onCancel: () => void }
interface ListProps { annotations: Annotation[]; sourceHash: string; onJump: (a: Annotation) => void; onEdit: (a: Annotation) => void; onDelete: (a: Annotation) => Promise<void> }

export function AnnotationEditor(props: EditorProps) {
  const [color, setColor] = useState(props.color);
  const [comment, setComment] = useState(props.comment);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const input = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    input.current?.focus();
    return () => { if (previous?.isConnected) previous.focus({ preventScroll: true }); };
  }, []);
  return <section className="rd-annotation-editor" role="dialog" aria-label={props.title} onKeyDown={(event) => {
    event.stopPropagation();
    if (event.key === "Escape" && !busy) { event.preventDefault(); props.onCancel(); }
  }}>
    <h3>{props.title}</h3>
    {props.quote && <blockquote>{props.quote}</blockquote>}
    <form onSubmit={async (event) => {
      event.preventDefault(); if (busy) return; setBusy(true); setError("");
      try { await props.onSave(color, comment); } catch (e) { setError(e instanceof Error ? e.message : "Could not save. Please retry."); } finally { setBusy(false); }
    }}>
      <label>Highlight color <select aria-label="Highlight color" value={color} disabled={busy} onChange={(event) => setColor(event.target.value as Annotation["color"])}>{annotationColors.map(c => <option key={c} value={c}>{c}</option>)}</select></label>
      <label>Note <textarea ref={input} aria-label="Note" value={comment} maxLength={10000} disabled={busy} onChange={(event) => setComment(event.target.value)} placeholder="Your reflection (optional)" /></label>
      {error && <p role="alert">{error}</p>}
      <div className="rd-annotation-actions"><button type="submit" disabled={busy}>{busy ? "Saving…" : "Save"}</button><button type="button" disabled={busy} onClick={props.onCancel}>Cancel</button></div>
    </form>
  </section>;
}

export function HighlightsList({ annotations, sourceHash, onJump, onEdit, onDelete }: ListProps) {
  const [confirm, setConfirm] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  return <section className="rd-highlights" aria-label="My highlights">
    <h3>My highlights <span>({annotations.length})</span></h3>
    {!annotations.length && <p>Select text in the document to save a highlight or note.</p>}
    <p role="status">{message}</p>{error && <p role="alert">{error}</p>}
    <ol>{[...annotations].sort((a,b) => a.page-b.page || a.created_at?.localeCompare(b.created_at)).map(a => {
      const mismatch = !!a.anchor && (!sourceHash || a.anchor.source_hash !== sourceHash);
      return <li key={a.id} data-annotation-id={a.id}>
        <div className="rd-annotation-meta">Page {a.page} · {a.color} · {a.type}</div>
        {a.anchor ? <blockquote>{a.anchor.exact}</blockquote> : <p>Legacy rectangle — no saved quote</p>}
        {a.comment && <p className="rd-annotation-note">{a.comment}</p>}
        {mismatch && <p>Source changed — this saved mark is preserved, but cannot be located or modified.</p>}
        <div className="rd-annotation-actions">
          <button type="button" disabled={mismatch || busy} onClick={() => onJump(a)}>Jump to page {a.page}</button>
          <button type="button" disabled={mismatch || busy} onClick={() => onEdit(a)}>Edit</button>
          <button type="button" disabled={!a.anchor || busy} onClick={async () => {
            setError(""); try { await navigator.clipboard.writeText(a.anchor!.exact); setMessage("Quote copied"); } catch { setError("Could not copy. Select and copy the quote manually."); }
          }}>Copy quote</button>
          <button type="button" disabled={mismatch || busy} onClick={() => setConfirm(a.id)}>Delete</button>
        </div>
        {confirm === a.id && <div role="group" aria-label="Confirm highlight deletion"><p>Delete this highlight and its note?</p>
          <button type="button" disabled={busy || mismatch} onClick={async () => {
            setBusy(true); setError(""); try { await onDelete(a); setConfirm(null); setMessage("Highlight deleted"); } catch(e) {setError(e instanceof Error ? e.message : "Could not delete. Please retry.");} finally {setBusy(false);}
          }}>Confirm delete</button><button type="button" disabled={busy} onClick={() => setConfirm(null)}>Keep highlight</button>
        </div>}
      </li>;
    })}</ol>
  </section>;
}
