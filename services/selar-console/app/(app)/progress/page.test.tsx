import {act} from 'react';import {createRoot} from 'react-dom/client';import {it,expect,vi} from 'vitest';import Progress from './page';
it('does not invent retention when delayed observations are absent',async()=>{
 (globalThis as typeof globalThis & {IS_REACT_ACT_ENVIRONMENT:boolean}).IS_REACT_ACT_ENVIRONMENT=true;
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({warmup:2,reading_check:1,review:0,delayed_unassisted:0,delayed_scored:0,delayed_mean_score:null,estimated_recall:null,streak:0,due:0,exposed:1,unscored:0}),{headers:{'Content-Type':'application/json'}})));
 const el=document.createElement('div');const root=createRoot(el);await act(async()=>root.render(<Progress/>));
 expect(el.textContent).toContain('No delayed recall observations yet');expect(el.textContent).toContain('Not estimated');expect(el.textContent).toContain('Formal study results');expect(el.textContent).not.toContain('0% retention');
 await act(async()=>root.unmount());vi.unstubAllGlobals();
});
