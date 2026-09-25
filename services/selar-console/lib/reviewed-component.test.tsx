import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ReviewAssertion } from "../components/ReviewAssertion";
import type { MentalLinkReviewPreview } from "./api";

const preview = {id:"link-id",revision:1,status:"confirmed",link_type:"concept_overlap",source_document_id:"source-id",target_document_id:"target-id",source_document_title:"New",target_document_title:"Prior",source_quote:"gradient descent optimization",target_quote:"Gradient descent optimization",source_locator:{page:2},target_locator:{page:3},history:[{revision:1,action:"confirmed",before_status:"candidate",after_status:"confirmed",reason:"",occurred_at:"2026-09-25T00:00:00Z"}]} as MentalLinkReviewPreview;
describe("assertion preview",()=>{
 it("shows two navigable witnesses, revision, audit and keyboard-native controls",()=>{
  const html=renderToStaticMarkup(<ReviewAssertion preview={preview} label="" reason="" busy={false} onLabel={()=>{}} onReason={()=>{}} onAct={()=>{}} />);
  expect(html).toContain("/reader?docId=source-id&amp;page=2");
  expect(html).toContain("/reader?docId=target-id&amp;page=3");
  expect(html).toContain("Revision 1");
  expect(html).toContain("confirmed");
  expect(html).toContain("<button");
 });
});
