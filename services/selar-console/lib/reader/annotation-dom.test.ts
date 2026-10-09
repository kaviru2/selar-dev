import { expect, it, vi } from "vitest";
import { captureSelectionAnchor, annotationBoxes } from "./annotation-dom";
import type { Annotation } from "@/lib/api";
it("captures nested find marks as original text and excludes other pages",()=>{
 const page=document.createElement("div");page.innerHTML='<div class="textLayer"><span>Before <mark>quoted</mark> text after</span></div>';
 const range=document.createRange();range.setStart(page.querySelector("mark")!.firstChild!,0);range.setEnd(page.querySelector("span")!.lastChild!,5);
 expect(captureSelectionAnchor(page,range,"hash")).toMatchObject({exact:"quoted text",start:7,end:18,prefix:"Before ",suffix:" after"});
 const outside=document.createTextNode("elsewhere");document.body.append(page,outside);range.setEnd(outside,3);expect(captureSelectionAnchor(page,range,"hash")).toBeNull();page.remove();outside.remove();
});
it("uses scale-free fallback for old rectangles and never paints source mismatches",()=>{
 const page=document.createElement("div"); const a={bbox:[{x:.1,y:.2,w:.3,h:.04}]} as Annotation;
 expect(annotationBoxes(page,a,"hash")).toEqual(a.bbox);
 a.anchor={version:1,source_hash:"old",exact:"quote",prefix:"",suffix:"",start:0,end:5};expect(annotationBoxes(page,a,"new")).toEqual([]);
});
it("reanchors to current range rectangles after zoom instead of stale geometry",()=>{
 const page=document.createElement("div");page.innerHTML='<div class="textLayer"><span>Before quote after</span></div>';
 const a={bbox:[{x:.8,y:.8,w:.1,h:.1}],anchor:{version:1,source_hash:"hash",exact:"quote",prefix:"Before ",suffix:" after",start:7,end:12}} as Annotation;
 vi.spyOn(page,"getBoundingClientRect").mockReturnValue({left:0,top:0,width:1000,height:2000} as DOMRect);
 Object.defineProperty(Range.prototype,"getClientRects",{configurable:true,value:()=>[{left:100,top:400,right:400,bottom:440,width:300,height:40}]});
 expect(annotationBoxes(page,a,"hash")).toEqual([{x:.1,y:.2,w:.3,h:.02}]);
 delete (Range.prototype as {getClientRects?:unknown}).getClientRects;
});
