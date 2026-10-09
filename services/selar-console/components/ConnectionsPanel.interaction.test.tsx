import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {ConnectionsPanel} from './ConnectionsPanel';
import type {MentalModelLink} from '@/lib/api';
const link={id:'link',source_document_id:'a',target_document_id:'b',source_document_title:'A',target_document_title:'B',source_quote:'Exact A',target_quote:'Exact B',source_locator:{page:1},target_locator:{page:2},status:'candidate',revision:0} as MentalModelLink;
let root:Root,el:HTMLDivElement,fetcher:ReturnType<typeof vi.fn>,reload:ReturnType<typeof vi.fn>;
async function render(){await act(async()=>{root.render(<ConnectionsPanel docId="a" links={[link]} loading={false} mentalModel={null} reloadLinks={reload}/>);});}
async function click(name:string){await act(async()=>{Array.from(el.querySelectorAll('button')).find(b=>b.textContent===name)!.click();});}
beforeEach(()=>{(globalThis as typeof globalThis & {IS_REACT_ACT_ENVIRONMENT:boolean}).IS_REACT_ACT_ENVIRONMENT=true;el=document.createElement('div');root=createRoot(el);reload=vi.fn(async()=>[]);fetcher=vi.fn(async()=>new Response(JSON.stringify({...link,history:[]}),{headers:{'Content-Type':'application/json'}}));vi.stubGlobal('fetch',fetcher);});
afterEach(async()=>{await act(async()=>root.unmount());vi.unstubAllGlobals();});
it('surfaces neutral reflection, no keep/reject or truth assertion',async()=>{
 await render();expect(el.textContent).toContain('Learning connections');expect(el.textContent).toContain('What similarities or differences');
 expect(el.textContent).not.toMatch(/keep this|confirm|not a real link|to review|%/i);
 expect(el.textContent).not.toContain('Exact A');
 await click('Compare passages');expect(el.textContent).toContain('Exact A');expect(el.textContent).toContain('Exact B');
 expect(fetcher.mock.calls.every(([,init])=>!init?.method||init.method==='GET')).toBe(true);
});
it('flags quietly and reads back without creating a relationship',async()=>{
 await render();await click('This link is wrong');expect(reload).toHaveBeenCalled();
 expect(fetcher.mock.calls.some(([url,init])=>url==='/api/mental-model-links/link/flag'&&init.method==='POST')).toBe(true);
 expect(fetcher.mock.calls.some(([url])=>String(url).endsWith('/respond'))).toBe(false);
});
it('fails closed without both located witnesses',async()=>{
 fetcher.mockImplementation(async()=>new Response(JSON.stringify({...link,target_quote:''}),{headers:{'Content-Type':'application/json'}}));
 await render();expect(el.textContent).toContain('Source evidence unavailable');expect(el.querySelector('textarea')).toBeNull();
});
