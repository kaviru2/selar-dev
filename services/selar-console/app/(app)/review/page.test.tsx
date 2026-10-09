import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect,it,vi } from 'vitest';
import DailyReview from './page';
it('shows a truthful empty daily queue and explicit UTC streak',async()=>{
 (globalThis as typeof globalThis & {IS_REACT_ACT_ENVIRONMENT:boolean}).IS_REACT_ACT_ENVIRONMENT=true;
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({items:[],streak:2}),{headers:{'Content-Type':'application/json'}})));
 const el=document.createElement('div');const root=createRoot(el);
 await act(async()=>{root.render(<DailyReview/>);});
 expect(el.textContent).toContain('Nothing due');expect(el.textContent).toContain('2-day');expect(el.textContent).toContain('UTC');
 await act(async()=>root.unmount());vi.unstubAllGlobals();
});
