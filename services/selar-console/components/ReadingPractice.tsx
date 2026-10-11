"use client";
import { useEffect, useState, type ReactNode } from 'react';
import Link from 'next/link';
import { quizApi } from '@/lib/quiz/client';
import { clearPracticeCache } from '@/lib/practice-cache';
import { buttonClass } from '@/components/ui/Button';

export interface PracticeItem { id:string; document_id?:string; question:string; label:string }
interface Result { status:string; streak?:number; message?:string; items?:PracticeItem[]; feedback?:{score:number|null; provisional:boolean; feedback:string}; quote?:string; answer?:string; locator?:unknown }
export const practiceRPC = (data:Record<string,unknown>) => quizApi<Result>('/api/practice',{method:'POST',body:JSON.stringify(data)});

export function PracticeCard({item,phase,onDone}:{item:PracticeItem;phase:'warmup'|'reading_check'|'review';onDone?:()=>void}) {
 const [response,setResponse]=useState(''); const [result,setResult]=useState<Result|null>(null);
 const [busy,setBusy]=useState(false); const [error,setError]=useState('');
 const [key]=useState(()=>crypto.randomUUID()); const [exposed,setExposed]=useState(phase==='reading_check');
 async function submit(){
  setBusy(true);setError('');
  try {const value=await practiceRPC({action:'attempt',item_id:item.id,request_key:key,response,phase,exposed});
   if(value.status!=='recorded') throw new Error(value.status==='conflict'?'This saved attempt has different contents. Start a new attempt.':'Source changed or practice unavailable; reload.');
   clearPracticeCache();setResult(value);onDone?.();
  } catch(e){setError(e instanceof Error?e.message:'Practice unavailable; retry');} finally {setBusy(false);}
 }
 return <section aria-label="Practice question" className="ui-card practice-card">
  <small className="practice-label">{item.label} · not a study test</small><h3 className="practice-question">{item.question}</h3>
  {!result ? <><label className="practice-field">Your recall<textarea aria-label="Your recall" value={response} onChange={e=>setResponse(e.target.value)} disabled={busy}/></label>
   <label className="practice-check"><input type="checkbox" checked={exposed} onChange={e=>setExposed(e.target.checked)} disabled={phase==='reading_check'||busy}/> I used the source or a hint</label>
   <div className="practice-actions"><button type="button" className={buttonClass({variant:'primary',size:'sm'})} onClick={submit} disabled={busy||!response.trim()}>{busy?'Checking…':'Check my recall'}</button></div></> : <div role="status" className="practice-result">
    <strong>{result.feedback?.provisional?'AI feedback · not scored':'AI practice feedback'}</strong>
    <p>{result.feedback?.feedback}</p><p>Reference answer: {result.answer}</p>
    <blockquote>{result.quote}</blockquote><small>Source locator: {JSON.stringify(result.locator)}. A source match and AI feedback do not establish mastery.</small>
   </div>}
  {error&&<p role="alert" className="practice-error">{error}</p>}
 </section>;
}

export function ReadingPractice({documentId,children,onReadingChange}:{documentId:string;children:ReactNode;onReadingChange?:(visible:boolean)=>void}) {
 const [stage,setStage]=useState<'warmup'|'reading'|'check'>('warmup');
 const [priorKnowledge,setPriorKnowledge]=useState('');
 useEffect(()=>{onReadingChange?.(Boolean(documentId)&&stage==='reading');return()=>onReadingChange?.(false);},[documentId,stage,onReadingChange]);
 const [items,setItems]=useState<PracticeItem[]>([]);const [message,setMessage]=useState('Loading practice…');
 async function load(){
  setMessage('Loading practice…');
  try {const value=await practiceRPC({action:'generate',document_id:documentId});setItems(value.items||[]);setMessage(value.message||(!value.items?.length?'Practice unavailable; retry after ingestion.':''));}
  catch{setMessage('Practice unavailable; retry. You can continue reading.');}
 }
 useEffect(()=>{
  if(!documentId)return;
  let active=true;
  practiceRPC({action:'generate',document_id:documentId}).then(value=>{
   if(!active)return;
   setItems(value.items||[]);
   setMessage(value.message||(!value.items?.length?'Practice unavailable; retry after ingestion.':''));
  }).catch(()=>{if(active)setMessage('Practice unavailable; retry. You can continue reading.');});
  return()=>{active=false;};
 },[documentId]);
 if(!documentId)return <>{children}</>;
 if(stage==='reading')return <><div className="practice-bar"><button type="button" className={buttonClass({size:'sm'})} onClick={()=>setStage('check')}>End-reading check</button> <Link className="practice-link" href="/review">Daily review</Link></div>{children}</>;
 return <main className="practice-page" aria-label="Reading practice"><div className="practice-inner">
  <span className="ui-eyebrow">Reading practice</span>
  <h2 className="practice-title">{stage==='warmup'?'Before reading':'End-reading check'}</h2>
  <p className="practice-lede">{stage==='warmup'?'Optional preparation, not a test. You do not need to know the reading yet. These notes stay on this screen and are not saved or scored.':'Put the reading aside and try to recall the main ideas. This immediate practice is not a delayed retention test.'}</p>
  {stage==='warmup' ? <section className="ui-card practice-card" aria-label="Prior knowledge">
   <label className="practice-field">What, if anything, do you already know about this reading&apos;s topic?
    <textarea aria-label="What you already know" value={priorKnowledge} onChange={e=>setPriorKnowledge(e.target.value)} placeholder="A brief note is enough. You can leave this blank."/>
   </label>
   <div className="practice-actions"><button type="button" className={buttonClass({size:'sm'})} onClick={()=>setStage('reading')}>I don&apos;t know yet</button></div>
  </section> : <>
   <p className="practice-lede">Optional AI-generated practice, not a study test.</p>
   {message&&<p role="status" className="practice-status">{message}</p>}
   {items.map(item=><PracticeCard key={`check:${item.id}`} item={item} phase="reading_check"/>)}
   {!items.length&&<button type="button" className={buttonClass({size:'sm'})} onClick={load}>Retry practice</button>}
  </>}
  <div className="practice-actions">
  <button type="button" className={buttonClass({variant:'primary',size:'sm'})} onClick={()=>setStage('reading')}>Continue reading</button> <Link className="practice-link" href="/review">Daily review</Link></div>
 </div></main>;
}
