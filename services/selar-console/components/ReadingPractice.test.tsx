import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { ReadingPractice } from './ReadingPractice';

it('hides reading until warm-up or skip and exposes an end-reading check', async () => {
 (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT=true;
 const el=document.createElement('div'); const root=createRoot(el);
 const fetcher=vi.fn(async()=>new Response(JSON.stringify({status:'ready',items:[{id:'i',question:'Recall the method',label:'AI-generated practice'}]}),{headers:{'Content-Type':'application/json'}}));
 vi.stubGlobal('fetch',fetcher);
 const visible=vi.fn();
 await act(async()=>{root.render(<ReadingPractice documentId="d" onReadingChange={visible}><p>Private source passage</p></ReadingPractice>);});
 expect(visible).not.toHaveBeenCalledWith(true);
 expect(el.textContent).toContain('Before reading');
 expect(el.textContent).not.toContain('Private source passage');
 const skip=Array.from(el.querySelectorAll('button')).find(b=>b.textContent==='Continue reading');
 await act(async()=>skip!.click());
 expect(el.textContent).toContain('Private source passage');
 expect(visible).toHaveBeenLastCalledWith(true);
 expect(el.textContent).toContain('End-reading check');
 await act(async()=>root.unmount()); vi.unstubAllGlobals();
});
