"use client";
import { useEffect, useRef, useState } from "react";
import type { MentalLinkReviewPreview } from "@/lib/api";
import { readerWitnessURL, reviewActions, type ReviewAction } from "@/lib/reviewed-links";

function EvidenceReflection({ preview }: { preview: MentalLinkReviewPreview }) {
 const [explanation, setExplanation] = useState("");
 const [recall, setRecall] = useState("");
 const [recalling, setRecalling] = useState(false);
 const [revisited, setRevisited] = useState(false);
 const [discarded, setDiscarded] = useState(false);
 const explanationRef = useRef<HTMLTextAreaElement>(null);
 const recallRef = useRef<HTMLTextAreaElement>(null);
 const quotesRef = useRef<HTMLDivElement>(null);
 useEffect(() => { if (recalling) recallRef.current?.focus(); else if (revisited) quotesRef.current?.focus(); }, [recalling, revisited]);
 return <section aria-label="Optional evidence reflection">
  {!recalling && <div ref={quotesRef} tabIndex={revisited ? -1 : undefined} aria-label="Source quotes for comparison">
   <p><strong>{preview.source_document_title}</strong>: <q>{preview.source_quote}</q> <a href={readerWitnessURL(preview.source_document_id,preview.source_locator)}>Open source in reader</a></p>
   <p><strong>{preview.target_document_title}</strong>: <q>{preview.target_quote}</q> <a href={readerWitnessURL(preview.target_document_id,preview.target_locator)}>Open target in reader</a></p>
  </div>}
  <p>Optional: explain in your own words how these two claims connect. This draft is not saved or scored; it is not a retrieval result.</p>
  <label>Your explanation of how the claims connect<textarea ref={explanationRef} aria-label="Your explanation of how the claims connect" placeholder="Compare the two quotes in your own words; leave blank if you prefer." value={explanation} onChange={event=>{ setDiscarded(false); setExplanation(event.target.value); }} /></label>
  <button type="button" onClick={() => { setExplanation(""); setRecall(""); setDiscarded(true); explanationRef.current?.focus(); }}>Discard drafts</button>
  <p>For an optional self-check, try recalling the relationship before revisiting the quotes here. Quotes may still be visible elsewhere in the reader; this is not a blinded assessment.</p>
  {!recalling && !revisited && <button type="button" onClick={() => { setDiscarded(false); setRecalling(true); }}>Try recalling the relationship</button>}
  {(recalling || revisited) && <label>Your optional recall of the relationship<textarea ref={recallRef} aria-label="Your optional recall of the relationship" autoComplete="off" placeholder="Recall in your own words, or leave blank." value={recall} onChange={event=>setRecall(event.target.value)} /></label>}
  {recalling && <button type="button" onClick={() => { setRevisited(true); setRecalling(false); }}>Revisit source quotes</button>}
  <p role="status" aria-live="polite">{discarded ? "Draft discarded from this review. No answer was saved." : recalling ? "Source quotes hidden in this review; you can revisit them at any time." : revisited ? "Source quotes shown again for comparison. No answer was scored or saved." : "Source quotes shown for comparison."}</p>
 </section>;
}

export function ReviewAssertion({preview,documentId,label,reason,busy,onLabel,onReason,onAct}:{
 preview:MentalLinkReviewPreview;documentId?:string;label:string;reason:string;busy:boolean;
 onLabel:(value:string)=>void;onReason:(value:string)=>void;onAct:(action:ReviewAction)=>void;
}) {
 const headingRef = useRef<HTMLHeadingElement>(null);
 useEffect(() => { headingRef.current?.focus(); }, [preview.id]);
 const hasLocator = (locator: MentalLinkReviewPreview["source_locator"]) => locator && (Number.isInteger(locator.page) || Number.isInteger(locator.block_index));
 if (!preview.id || !preview.source_document_id || !preview.target_document_id || !preview.source_quote?.trim() || !preview.target_quote?.trim() || !hasLocator(preview.source_locator) || !hasLocator(preview.target_locator)) {
  return <section aria-label="Grounded assertion review" className="connection-evidence"><p role="alert">Two exact source quotes and their locations are unavailable. This assertion cannot be reviewed or used for reflection. Refresh the evidence preview.</p></section>;
 }
 const actions=reviewActions(preview.status,preview.revision);
 return <section aria-label="Grounded assertion review" className="connection-evidence">
  <h3 ref={headingRef} tabIndex={-1}>Review concept overlap · Revision {preview.revision}</h3>
  <p>This is exact concept overlap in two documents, not proof of extension or contradiction. Your review is not a retrieval success.</p>
  <EvidenceReflection key={`${documentId ?? preview.source_document_id}:${preview.id}`} preview={preview} />
  {preview.user_label && <p>Human note: {preview.user_label}</p>}
  {actions.includes("relabeled") && <label>Correction note (not a new relation type)<input value={label} maxLength={160} onChange={event=>onLabel(event.target.value)} /></label>}
  {(actions.includes("rejected") || actions.includes("retracted") || actions.includes("rolled_back")) && <label>Reason for rejection, retraction or rollback<input value={reason} maxLength={500} onChange={event=>onReason(event.target.value)} /></label>}
  <div className="foot">{actions.map(action=><button key={action} type="button" disabled={busy || (action==="relabeled" && !label.trim()) || (["rejected","retracted","rolled_back"].includes(action) && !reason.trim())} onClick={()=>onAct(action)}>{action.replaceAll("_"," ")}</button>)}</div>
  <details><summary>Review history ({preview.history.length})</summary><ol>{preview.history.map(event=><li key={event.revision}>Revision {event.revision}: {event.action} · {event.before_status} → {event.after_status}{event.reason ? ` · ${event.reason}` : ""}</li>)}</ol></details>
 </section>;
}
