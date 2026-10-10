"use client";
import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { Annotation } from "@/lib/api";
import { Icon } from "@/components/ui/Icon";

type Color = Annotation["color"];
export const annotationColors: Color[] = ["yellow", "wheat", "coral", "sage"];
/** Human names for the stored colour keys (the API keeps its original keys). */
export const annotationColorLabels: Record<Color, string> = { yellow: "Yellow", wheat: "Orange", coral: "Pink", sage: "Green" };

/** Map the Settings › Reader highlight colour onto a colour the annotation API stores. */
export function annotationColorFor(preference: string | undefined): Color {
  switch (preference) {
    case "yellow": case "wheat": case "coral": case "sage": return preference;
    case "green": return "sage";
    case "pink": return "coral";
    case "orange": return "wheat";
    default: return "yellow";
  }
}

/** ⌘↵ / Ctrl↵ submits, as in most note editors. */
export function isSubmitChord(event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey">): boolean {
  return event.key === "Enter" && (event.metaKey || event.ctrlKey);
}

const DRAFT_PREFIX = "selar.noteDraft.";
function readDraft(key?: string): string | null {
  if (!key) return null;
  try { return window.sessionStorage.getItem(DRAFT_PREFIX + key); } catch { return null; }
}
function writeDraft(key: string | undefined, value: string | null) {
  if (!key) return;
  try {
    if (value === null) window.sessionStorage.removeItem(DRAFT_PREFIX + key);
    else window.sessionStorage.setItem(DRAFT_PREFIX + key, value);
  } catch { /* storage unavailable: the editor still works without drafts */ }
}

/** One-click colour chips: a radio group; arrow keys move between colours. */
export function ColorSwatches({ value, onChange, onPick, disabled, label = "Highlight color" }: {
  value: Color; onChange?: (color: Color) => void; onPick?: (color: Color) => void; disabled?: boolean; label?: string;
}) {
  const refs = useRef<Array<HTMLButtonElement | null>>([]);
  function onKeyDown(event: ReactKeyboardEvent, index: number) {
    const step = event.key === "ArrowRight" || event.key === "ArrowDown" ? 1 : event.key === "ArrowLeft" || event.key === "ArrowUp" ? -1 : 0;
    if (!step) return;
    event.preventDefault();
    const next = (index + step + annotationColors.length) % annotationColors.length;
    onChange?.(annotationColors[next]);
    refs.current[next]?.focus();
  }
  return <div className="rd-swatches" role="radiogroup" aria-label={label}>
    {annotationColors.map((color, index) => (
      <button key={color} ref={(el) => { refs.current[index] = el; }} type="button" role="radio" aria-checked={value === color}
        aria-label={annotationColorLabels[color]} title={annotationColorLabels[color]} tabIndex={value === color ? 0 : -1}
        className={`rd-swatch c-${color}`} disabled={disabled}
        onClick={() => { onChange?.(color); onPick?.(color); }} onKeyDown={(event) => onKeyDown(event, index)} />
    ))}
  </div>;
}

interface EditorProps {
  title: string; quote?: string; page?: number; color: Color; comment: string;
  /** Unsaved text survives accidental closes (navigation, reload) under this key. */
  draftKey?: string;
  onSave: (color: Color, comment: string) => Promise<void>; onCancel: () => void;
  /** Present only when editing a saved mark. */
  onDelete?: () => Promise<void>;
}
interface ListProps {
  annotations: Annotation[]; sourceHash: string; currentPage?: number;
  onJump: (a: Annotation) => void; onEdit: (a: Annotation) => void; onDelete: (a: Annotation) => Promise<void>; onClose?: () => void;
}

export function AnnotationEditor(props: EditorProps) {
  const { draftKey } = props;
  const [initialDraft] = useState(() => {
    const draft = readDraft(draftKey);
    return draft !== null && draft !== props.comment ? draft : null;
  });
  const [color, setColor] = useState(props.color);
  const [comment, setComment] = useState(initialDraft ?? props.comment);
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [error, setError] = useState("");
  const input = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const field = input.current;
    if (field) { field.focus({ preventScroll: true }); field.setSelectionRange(field.value.length, field.value.length); }
    return () => { if (previous?.isConnected) previous.focus({ preventScroll: true }); };
  }, []);
  async function save() {
    if (busy) return;
    setBusy(true); setError("");
    try {
      await props.onSave(color, comment.trim());
      writeDraft(draftKey, null);
    } catch (e) { setError(e instanceof Error ? e.message : "Could not save. Please retry."); setBusy(false); return; }
    setBusy(false);
  }
  function cancel() {
    if (busy) return;
    writeDraft(draftKey, null);
    props.onCancel();
  }
  return <section className="rd-annotation-editor rd-sheet" role="dialog" aria-label={props.title} onKeyDown={(event) => {
    event.stopPropagation();
    if (event.key === "Escape") { event.preventDefault(); cancel(); }
    else if (isSubmitChord(event.nativeEvent)) { event.preventDefault(); void save(); }
  }}>
    <header className="rd-sheet-head">
      <h3>{props.title}</h3>
      {props.page ? <span className="rd-sheet-sub">Page {props.page}</span> : null}
      <button type="button" className="rd-sheet-close" aria-label="Close editor" disabled={busy} onClick={cancel}><Icon name="x" size={13} /></button>
    </header>
    {props.quote && <blockquote className={`rd-quote c-${color}`}>{props.quote}</blockquote>}
    <form onSubmit={(event) => { event.preventDefault(); void save(); }}>
      <div className="rd-field-row"><span className="rd-field-label">Color</span><ColorSwatches value={color} onChange={setColor} disabled={busy} /></div>
      <label className="rd-field">
        <span className="rd-field-label">Note</span>
        <textarea ref={input} aria-label="Note" value={comment} maxLength={10000} disabled={busy} rows={4}
          onChange={(event) => { setComment(event.target.value); writeDraft(draftKey, event.target.value); }}
          placeholder="Write a note in your own words (optional)" />
      </label>
      {initialDraft !== null && <p className="rd-status-inline" role="status">Restored your unsaved note.</p>}
      {error && <p role="alert">{error}</p>}
      <div className="rd-annotation-actions">
        {props.onDelete && (confirmDelete
          ? <span className="rd-confirm-inline" role="group" aria-label="Confirm highlight deletion">
            <button type="button" className="rd-danger" disabled={busy} onClick={async () => {
              setBusy(true); setError("");
              try { await props.onDelete!(); writeDraft(draftKey, null); } catch (e) { setError(e instanceof Error ? e.message : "Could not delete. Please retry."); setBusy(false); }
            }}>Confirm delete</button>
            <button type="button" disabled={busy} onClick={() => setConfirmDelete(false)}>Keep</button>
          </span>
          : <button type="button" className="rd-ghost-danger" disabled={busy} onClick={() => setConfirmDelete(true)}>Delete highlight</button>)}
        <span className="rd-actions-spacer" />
        <span className="rd-kbd-hint" aria-hidden="true">⌘↵ to save</span>
        <button type="button" disabled={busy} onClick={cancel}>Cancel</button>
        <button type="submit" className="rd-primary" disabled={busy}>{busy ? "Saving…" : "Save"}</button>
      </div>
    </form>
  </section>;
}

export function HighlightsList({ annotations, sourceHash, currentPage, onJump, onEdit, onDelete, onClose }: ListProps) {
  const [confirm, setConfirm] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  // Reading order: page, then position on the page, then creation time.
  const sorted = [...annotations].sort((a, b) => a.page - b.page || (a.anchor?.start ?? 0) - (b.anchor?.start ?? 0) || (a.created_at || "").localeCompare(b.created_at || ""));
  return <aside className="rd-highlights" aria-label="My highlights" onKeyDown={(event) => {
    if (event.key === "Escape" && onClose) { event.stopPropagation(); event.preventDefault(); onClose(); }
  }}>
    <header className="rd-sheet-head">
      <h3>My highlights <span className="rd-sheet-sub">{annotations.length}</span></h3>
      {onClose && <button type="button" className="rd-sheet-close" aria-label="Close highlights" onClick={onClose}><Icon name="x" size={13} /></button>}
    </header>
    {!annotations.length && <div className="rd-empty-hint">
      <Icon name="highlight" size={18} />
      <p><strong>No highlights yet</strong></p>
      <p>Select text on the page, then pick a colour to highlight it or choose Note to add your own words.</p>
    </div>}
    <p role="status" className="rd-status">{message}</p>{error && <p role="alert">{error}</p>}
    <ol>{sorted.map(a => {
      const mismatch = !!a.anchor && (!sourceHash || a.anchor.source_hash !== sourceHash);
      const here = currentPage === a.page;
      return <li key={a.id} data-annotation-id={a.id} className={here ? "here" : undefined}>
        <button type="button" className={`rd-hl-card c-${a.color}`} disabled={mismatch || busy} aria-label={`Jump to page ${a.page}`} onClick={() => onJump(a)}>
          <span className="rd-annotation-meta"><span className={`rd-dot c-${a.color}`} aria-hidden="true" /><span className="ui-visually-hidden">{annotationColorLabels[a.color] ?? a.color} highlight, </span>Page {a.page}{a.comment ? " · Note" : ""}</span>
          {a.anchor ? <span className="rd-quote-text">{a.anchor.exact}</span> : <span className="rd-quote-text muted">Legacy rectangle — no saved quote</span>}
          {a.comment && <span className="rd-annotation-note">{a.comment}</span>}
        </button>
        {mismatch && <p className="rd-warn">Source changed — this saved mark is preserved, but cannot be located or modified.</p>}
        <div className="rd-annotation-actions compact">
          <button type="button" disabled={mismatch || busy} onClick={() => onEdit(a)}>Edit</button>
          <button type="button" disabled={!a.anchor || busy} onClick={async () => {
            setError(""); try { await navigator.clipboard.writeText(a.anchor!.exact); setMessage("Quote copied"); } catch { setError("Could not copy. Select and copy the quote manually."); }
          }}>Copy quote</button>
          <button type="button" className="rd-ghost-danger" disabled={mismatch || busy} onClick={() => setConfirm(a.id)}>Delete</button>
        </div>
        {confirm === a.id && <div className="rd-confirm" role="group" aria-label="Confirm highlight deletion"><p>Delete this highlight and its note?</p>
          <button type="button" className="rd-danger" disabled={busy || mismatch} onClick={async () => {
            setBusy(true); setError(""); try { await onDelete(a); setConfirm(null); setMessage("Highlight deleted"); } catch (e) { setError(e instanceof Error ? e.message : "Could not delete. Please retry."); } finally { setBusy(false); }
          }}>Confirm delete</button><button type="button" disabled={busy} onClick={() => setConfirm(null)}>Keep highlight</button>
        </div>}
      </li>;
    })}</ol>
  </aside>;
}
