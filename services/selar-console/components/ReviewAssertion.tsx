"use client";
import type { MentalLinkReviewPreview } from "@/lib/api";
import { readerWitnessURL, reviewActions, type ReviewAction } from "@/lib/reviewed-links";

export function ReviewAssertion({preview,label,reason,busy,onLabel,onReason,onAct}:{
 preview:MentalLinkReviewPreview;label:string;reason:string;busy:boolean;
 onLabel:(value:string)=>void;onReason:(value:string)=>void;onAct:(action:ReviewAction)=>void;
}) {
 const actions=reviewActions(preview.status,preview.revision);
 return <section aria-label="Grounded assertion review" className="connection-evidence">
  <h3>Review concept overlap · Revision {preview.revision}</h3>
  <p>This is exact concept overlap in two documents, not proof of extension or contradiction. Your review is not a retrieval success.</p>
  <p><strong>{preview.source_document_title}</strong>: <q>{preview.source_quote}</q> <a href={readerWitnessURL(preview.source_document_id,preview.source_locator)}>Open source in reader</a></p>
  <p><strong>{preview.target_document_title}</strong>: <q>{preview.target_quote}</q> <a href={readerWitnessURL(preview.target_document_id,preview.target_locator)}>Open target in reader</a></p>
  {preview.user_label && <p>Human note: {preview.user_label}</p>}
  {actions.includes("relabeled") && <label>Correction note (not a new relation type)<input value={label} maxLength={160} onChange={event=>onLabel(event.target.value)} /></label>}
  {(actions.includes("rejected") || actions.includes("retracted") || actions.includes("rolled_back")) && <label>Reason for rejection, retraction or rollback<input value={reason} maxLength={500} onChange={event=>onReason(event.target.value)} /></label>}
  <div className="foot">{actions.map(action=><button key={action} type="button" disabled={busy || (action==="relabeled" && !label.trim()) || (["rejected","retracted","rolled_back"].includes(action) && !reason.trim())} onClick={()=>onAct(action)}>{action.replaceAll("_"," ")}</button>)}</div>
  <details><summary>Review history ({preview.history.length})</summary><ol>{preview.history.map(event=><li key={event.revision}>Revision {event.revision}: {event.action} · {event.before_status} → {event.after_status}{event.reason ? ` · ${event.reason}` : ""}</li>)}</ol></details>
 </section>;
}
