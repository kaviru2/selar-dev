import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AnnotationEditor, HighlightsList } from "./AnnotationTools";
import type { Annotation } from "@/lib/api";
let host: HTMLDivElement, root: Root;
beforeEach(()=>{(globalThis as typeof globalThis & {IS_REACT_ACT_ENVIRONMENT:boolean}).IS_REACT_ACT_ENVIRONMENT=true;host=document.createElement("div");document.body.append(host);root=createRoot(host)});
afterEach(()=>{act(()=>root.unmount());host.remove()});
const click=async(text:string)=>act(async()=>{Array.from(host.querySelectorAll("button")).find(b=>b.textContent===text)!.click()});
const mark={user_id:"owner",chunk_id:null,created_at:"",updated_at:"",id:"a",document_id:"d",page:2,color:"yellow",type:"highlight",comment:"note",bbox:[],anchor:{version:1,source_hash:"s",exact:"Exact quote",prefix:"",suffix:"",start:0,end:11}} as Annotation;
it("focuses a labeled non-prompt editor and keeps failed saves editable",async()=>{
 const save=vi.fn().mockRejectedValue(new Error("Offline"));const cancel=vi.fn();
 await act(async()=>root.render(<AnnotationEditor title="Edit highlight" quote="Exact quote" color="yellow" comment="note" onSave={save} onCancel={cancel}/>));
 expect(host.querySelector('textarea[aria-label="Note"]')).toBe(document.activeElement);
 await click("Save");expect(save).toHaveBeenCalledWith("yellow","note");expect(host.querySelector('[role="alert"]')?.textContent).toBe("Offline");
 await click("Cancel");expect(cancel).toHaveBeenCalledOnce();
});
it("lists real quotes, labels legacy marks, jumps, edits and confirms deletion",async()=>{
 const jump=vi.fn(),edit=vi.fn(),remove=vi.fn().mockResolvedValue(undefined);
 await act(async()=>root.render(<HighlightsList annotations={[mark,{...mark,id:"legacy",anchor:null}]} sourceHash="s" onJump={jump} onEdit={edit} onDelete={remove}/>));
 expect(host.textContent).toContain("Exact quote");expect(host.textContent).toContain("Legacy rectangle — no saved quote");
 await click("Jump to page 2");expect(jump).toHaveBeenCalledWith(mark);
 await click("Edit");expect(edit).toHaveBeenCalledWith(mark);
 await click("Delete");expect(remove).not.toHaveBeenCalled();await click("Confirm delete");expect(remove).toHaveBeenCalledWith(mark);
});
it("never enables jump/edit/delete when the source changed",async()=>{
 await act(async()=>root.render(<HighlightsList annotations={[mark]} sourceHash="changed" onJump={vi.fn()} onEdit={vi.fn()} onDelete={vi.fn()}/>));
 expect(host.textContent).toContain("Source changed");
 expect(Array.from(host.querySelectorAll("button")).filter(b=>/Jump|Edit|Delete/.test(b.textContent||"")).every(b=>b.disabled)).toBe(true);
});
