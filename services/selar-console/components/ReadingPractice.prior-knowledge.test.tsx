import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { ReadingPractice, PracticeCard } from './ReadingPractice';

it('keeps prior knowledge private and unscored without leaking the later retrieval question', async () => {
 (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT=true;
 const el=document.createElement('div'); const root=createRoot(el);
 const fetcher=vi.fn(async()=>new Response(JSON.stringify({status:'ready',items:[{id:'i',question:'What prevents a retry from creating duplicate payments?',label:'AI-generated practice'}]}),{headers:{'Content-Type':'application/json'}}));
 vi.stubGlobal('fetch',fetcher);
 try {
  await act(async()=>{root.render(<ReadingPractice documentId="d"><p>Private source passage</p></ReadingPractice>);});
  expect(el.textContent).not.toContain('What prevents a retry');
  expect(el.textContent).toContain('What, if anything, do you already know');
  expect(el.textContent).not.toContain('Check my recall');
  expect(el.textContent).toContain("I don't know yet");
  await act(async()=>Array.from(el.querySelectorAll('button')).find(b=>b.textContent==="I don't know yet")!.click());
  expect(el.textContent).toContain('Private source passage');
  await act(async()=>Array.from(el.querySelectorAll('button')).find(b=>b.textContent==='End-reading check')!.click());
  expect(el.textContent).toContain('What prevents a retry from creating duplicate payments?');
  expect(el.textContent).toContain('Check my recall');
  expect(fetcher.mock.calls).toHaveLength(1); // generation only; no prior-knowledge grading or persistence
 } finally {await act(async()=>root.unmount());vi.unstubAllGlobals();}
});

it('shows unscored feedback and its source only after submitting retrieval', async () => {
 (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT=true;
 const el=document.createElement('div'); document.body.append(el); const root=createRoot(el);
 const fetcher=vi.fn(async(_url:unknown,init?:RequestInit)=>{
  const request=JSON.parse(String(init?.body));
  expect(request.phase).toBe('reading_check'); expect(request.exposed).toBe(true);
  return new Response(JSON.stringify({status:'recorded',feedback:{score:null,provisional:true,feedback:'I could not assess this response reliably.'},answer:'An idempotency key',quote:'An idempotency key prevents duplicate payments.',locator:{page:2}}),{headers:{'Content-Type':'application/json'}});
 });
 vi.stubGlobal('fetch',fetcher);
 try {
  await act(async()=>root.render(<PracticeCard item={{id:'i',question:'What prevents duplicate payments?',label:'AI-generated practice'}} phase="reading_check"/>));
  expect(el.textContent).not.toContain('An idempotency key');
  const textarea=el.querySelector('textarea')!;
  await act(async()=>{
   Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(textarea,'A request identifier');
   textarea.dispatchEvent(new Event('input',{bubbles:true}));
  });
  await act(async()=>Array.from(el.querySelectorAll('button')).find(b=>b.textContent==='Check my recall')!.click());
  expect(fetcher).toHaveBeenCalledTimes(1);
  expect(el.textContent).toContain('AI feedback · not scored');
  expect(el.querySelector('blockquote')?.textContent).toBe('An idempotency key prevents duplicate payments.');
  expect(el.textContent).toContain('Reference answer: An idempotency key');
 } finally {await act(async()=>root.unmount());el.remove();vi.unstubAllGlobals();}
});
