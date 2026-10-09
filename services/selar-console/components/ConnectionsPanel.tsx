"use client";
import {useEffect,useState,type ReactNode} from 'react';
import {clientFetch,type DocumentMentalModel,type MentalLinkReviewPreview,type MentalModelLink} from '@/lib/api';
import {readerWitnessURL} from '@/lib/reviewed-links';
import {locatorLabel} from '@/lib/connection-guide';
export type OpenWitness=(docId:string,locator:{page?:number;block_index?:number}|undefined,quote:string)=>boolean;
export interface ConnectionsPanelProps {docId:string;links:MentalModelLink[];loading:boolean;mentalModel:DocumentMentalModel|null;reloadLinks:()=>Promise<MentalModelLink[]>;focusLinkId?:string|null;passageSection?:ReactNode;onOpenWitness?:OpenWitness}
export function isReviewable(p:MentalLinkReviewPreview|null):p is MentalLinkReviewPreview {
 const located=(l:{page?:number;block_index?:number}|undefined)=>Boolean(l&&(Number.isInteger(l.page)||Number.isInteger(l.block_index)));
 return Boolean(p?.id&&p.source_document_id&&p.target_document_id&&p.source_quote?.trim()&&p.target_quote?.trim()&&located(p.source_locator)&&located(p.target_locator));
}
export function ConnectionsPanel({docId,links,loading,mentalModel,reloadLinks,passageSection,onOpenWitness}:ConnectionsPanelProps){
 const active=links.filter(l=>!['rejected','archived'].includes(l.status));
 return <aside className="matches mental-panel cx-panel" aria-label="Connections"><h2>Learning connections</h2>
 <p className="cx-note">Prompts for reflection, not established relationships. Shared wording does not establish that one reading supports another. Opening or flagging a prompt never counts as recall success.</p>
 <div className="matches-body cx-body">{loading?<p>Loading connections…</p>:active.length?active.map(link=><Reflection key={`${docId}:${link.id}:${link.revision}`} link={link} reload={reloadLinks} onOpenWitness={onOpenWitness}/>):<p>No suggested connections yet. Prompts need two current, exact located passages.</p>}
 <details><summary>About this reading</summary>{mentalModel&&<><p>{mentalModel.main_claim}</p><small>AI-generated summary; compare with the source.</small></>}{passageSection}</details></div></aside>;
}
function Reflection({link,reload,onOpenWitness}:{link:MentalModelLink;reload:()=>Promise<MentalModelLink[]>;onOpenWitness?:OpenWitness}){
 const [preview,setPreview]=useState<MentalLinkReviewPreview|null>(null),[error,setError]=useState(''),[draft,setDraft]=useState(''),[show,setShow]=useState(false),[hidden,setHidden]=useState(false),[busy,setBusy]=useState(false);
 useEffect(()=>{let active=true;clientFetch<MentalLinkReviewPreview>(`/api/mental-model-links/${link.id}/preview`).then(p=>{if(active)setPreview(p);}).catch(()=>{if(active)setError('Source evidence unavailable');});return()=>{active=false;};},[link.id]);
 async function flag(){setBusy(true);try{await clientFetch(`/api/mental-model-links/${link.id}/flag`,{method:'POST'});await reload();setHidden(true);}catch{setError('Could not hide this prompt; reload and retry.');}finally{setBusy(false);}}
 if(hidden)return <p role="status">Prompt hidden.</p>;
 if(!isReviewable(preview))return <p role="status">{error||'Source evidence unavailable; no reflection prompt shown.'}</p>;
 return <section className="cx-card" aria-label="Connection reflection"><h3>{link.source_document_title} ↔ {link.target_document_title}</h3>
 <label>What similarities or differences do you remember between these readings?<textarea aria-label="Your reflection" value={draft} onChange={e=>setDraft(e.target.value)}/></label>
 <p className="cx-note">Optional reflection: not graded, not saved and not sent anywhere.</p>
 <button onClick={()=>setShow(!show)}>{show?'Hide passages':'Compare passages'}</button>
 {show&&(['source','target'] as const).map(side=><figure key={side}><figcaption>{preview[`${side}_document_title`]} · {locatorLabel(preview[`${side}_locator`])} · <a href={readerWitnessURL(preview[`${side}_document_id`],preview[`${side}_locator`])} onClick={e=>{if(onOpenWitness?.(preview[`${side}_document_id`],preview[`${side}_locator`],preview[`${side}_quote`]!))e.preventDefault();}}>Open passage</a></figcaption><blockquote>{preview[`${side}_quote`]}</blockquote></figure>)}
 <button className="cx-link" disabled={busy} onClick={flag}>This link is wrong</button>{error&&<p role="alert">{error}</p>}
 </section>;
}
