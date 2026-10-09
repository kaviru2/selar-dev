import { describe, expect, it } from "vitest";
import { makeTextAnchor, resolveTextAnchor } from "./annotation-anchor";

describe("persisted selection anchors", () => {
 it("captures exact source text, context and UTF-16 offsets, never a generated quote", () => {
  expect(makeTextAnchor("Before quoted text after", 7, 18, "sha")).toEqual({ version: 1, source_hash: "sha", exact: "quoted text", prefix: "Before ", suffix: " after", start: 7, end: 18 });
 });
 it("resolves repeated quotes by context after reflow and shifted offsets", () => {
  const anchor = makeTextAnchor("one quote end two quote finish",18,23,"sha")!;
  const match = resolveTextAnchor(["new one quote end ","two ","quote", " finish"], anchor, "sha");
  expect(match?.start).toEqual({segment:2,offset:0});
 });
 it("refuses changed or missing source binding and partial quote matches", () => {
  const anchor = makeTextAnchor("a long quoted selection with a changed ending",0,44,"sha")!;
  expect(resolveTextAnchor([anchor.exact],anchor,"other")).toBeNull();
  expect(resolveTextAnchor([anchor.exact],anchor,"")).toBeNull();
  expect(resolveTextAnchor(["a long quoted selection with a DIFFERENT ending"],anchor,"sha")).toBeNull();
 });
 it("does not create an anchor for missing source or invalid offsets", () => {
  expect(makeTextAnchor("text",0,4,"")).toBeNull();
  expect(makeTextAnchor("text",-1,4,"sha")).toBeNull();
 });
});
