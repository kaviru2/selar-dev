import { afterEach, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ArticleReader } from './ArticleReader';
import type { DocumentContent } from '@/lib/api';
import { uploadPDF } from '@/lib/upload';

afterEach(() => vi.unstubAllGlobals());

it.each([['notes.md','text/markdown'],['notes.txt','text/plain'],['notes.docx','application/vnd.openxmlformats-officedocument.wordprocessingml.document']])('uploads %s with correct MIME', async (name,mime) => {
 const fetcher = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({mode:'multipart'}))).mockResolvedValueOnce(new Response('{}',{status:202}));
 vi.stubGlobal('fetch',fetcher);
 await uploadPDF(new File(['synthetic'],name));
 expect(JSON.parse(fetcher.mock.calls[0][1].body).content_type).toBe(mime);
});

it('preserves heading level and ordered-list markers', () => {
 const content = {document:{id:'fixture',title:'Synthetic',source_type:'markdown'},assets:[],blocks:[{id:'h',block_index:0,kind:'heading',text:'Third level',metadata:{level:3}},{id:'l',block_index:1,kind:'list',text:'Evidence',metadata:{marker:'7.'}}]} as unknown as DocumentContent;
 const container = document.createElement('div'); container.innerHTML = renderToStaticMarkup(<ArticleReader content={content}/>);
 expect(container.querySelector('h3')?.textContent).toBe('Third level');
 expect(container.textContent).toContain('7. Evidence');
});

it('renders DOCX tables and TXT literally without interpreting Markdown', () => {
 const content = {document:{id:'fixture',title:'Synthetic',source_type:'docx'},assets:[],blocks:[{id:'b',block_index:0,kind:'table',text:'A\tB',metadata:{literal:true,rows:[['A','B'],['literal *stars*','value']]}}]} as unknown as DocumentContent;
 const container = document.createElement('div'); container.innerHTML = renderToStaticMarkup(<ArticleReader content={content}/>);
 expect(container.querySelectorAll('td').length).toBe(4);
 expect(container.textContent).toContain('literal *stars*');
 expect(container.querySelector('em')).toBeNull();
});

it('does not fetch remote Markdown images or execute raw HTML/unsafe URLs', () => {
 const content = {document:{id:'fixture',title:'Synthetic',source_type:'markdown',source_url:'javascript:alert(1)'},assets:[],blocks:[{id:'b',block_index:0,kind:'paragraph',text:'![tracking](https://tracker.invalid/pixel) <img src="https://tracker.invalid/raw"> [bad](javascript:alert(1))',metadata:{}}]} as unknown as DocumentContent;
 const container = document.createElement('div');
 container.innerHTML = renderToStaticMarkup(<ArticleReader content={content}/>);
 expect(container.querySelector('img')).toBeNull();
 expect(container.querySelector('a[href^="javascript:"]')).toBeNull();
});
