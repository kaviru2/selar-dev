"use client";
import { useEffect, useState, type ReactNode } from 'react';
import { quizApi } from '@/lib/quiz/client';

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
   setResult(value);onDone?.();
  } catch(e){setError(e instanceof Error?e.message:'Practice unavailable; retry');} finally {setBusy(false);}
 }
 return <section aria-label="Practice question" className="card" style={{padding:16,margin:'12px 0'}}>
  <small>{item.label} · not a study test</small><h3>{item.question}</h3>
  {!result ? <><label>Your recall<textarea aria-label="Your recall" value={response} onChange={e=>setResponse(e.target.value)} disabled={busy}/></label>
   <label><input type="checkbox" checked={exposed} onChange={e=>setExposed(e.target.checked)} disabled={phase==='reading_check'||busy}/> I used the source or a hint</label>
   <button onClick={submit} disabled={busy||!response.trim()}>{busy?'Checking…':'Check my recall'}</button></> : <div role="status">
    <strong>{result.feedback?.provisional?'Provisional / unscored AI feedback':'AI practice feedback'}</strong>
    <p>{result.feedback?.feedback}</p><p>Reference answer: {result.answer}</p>
    <blockquote>{result.quote}</blockquote><small>Source locator: {JSON.stringify(result.locator)}. A source match and AI feedback do not establish mastery.</small>
   </div>}
  {error&&<p role="alert">{error}</p>}
 </section>;
}

export function ReadingPractice({documentId,children,onReadingChange}:{documentId:string;children:ReactNode;onReadingChange?:(visible:boolean)=>void}) {
 const [stage,setStage]=useState<'warmup'|'reading'|'check'>('warmup');
 useEffect(()=>{onReadingChange?.(Boolean(documentId)&&stage==='reading');return()=>onReadingChange?.(false);},[documentId,stage,onReadingChange]);
 const [items,setItems]=useState<PracticeItem[]>([]);const [message,setMessage]=useState('Loading practice…');
 async function load(){
  setMessage('Loading practice…');
  try {const value=await practiceRPC({action:'generate',document_id:documentId});setItems(value.items||[]);setMessage(value.message||(!value.items?.length?'Practice unavailable; retry after ingestion.':''));}
  catch{setMessage('Practice unavailable; retry. You can continue reading.');}
 }
 useEffect(()=>{if(documentId) void load(); /* keyed by document in reader */},[documentId]); // eslint-disable-line react-hooks/exhaustive-deps
 if(!documentId)return <>{children}</>;
 if(stage==='reading')return <><div style={{padding:8}}><button onClick={()=>setStage('check')}>End-reading check</button> <a href="/review">Daily review</a></div>{children}</>;
 return <main style={{maxWidth:780,margin:'24px auto',padding:24,overflowY:'auto'}} aria-label="Reading practice">
  <h2>{stage==='warmup'?'Before reading':'End-reading check'}</h2>
  <p>{stage==='warmup'?'Try recalling what you already know before opening the source. Optional; you can skip.':'Close the reading and recall the main ideas. This immediate, source-exposed check is not delayed retention.'}</p>
  {message&&<p role="status">{message}</p>}
  {items.map(item=><PracticeCard key={`${stage}:${item.id}`} item={item} phase={stage==='warmup'?'warmup':'reading_check'}/>)}
  {!items.length&&<button onClick={load}>Retry practice</button>}
  <button onClick={()=>setStage('reading')}>Continue reading</button> <a href="/review">Daily review</a>
 </main>;
}
