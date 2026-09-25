import { describe, expect, it } from "vitest";
import { reviewActions, readerWitnessURL } from "./reviewed-links";

describe("grounded review controls", () => {
 it("only offers rollback for a previous revision and no repeated confirmation", () => {
  expect(reviewActions("candidate", 0)).toEqual(["confirmed", "relabeled", "rejected"]);
  expect(reviewActions("confirmed", 1)).toEqual(["relabeled", "retracted", "rolled_back"]);
  expect(reviewActions("rejected", 1)).toEqual(["rolled_back"]);
 });
 it("navigates exact document and locator back to the reader", () => {
  expect(readerWitnessURL("source-id", {page:3})).toBe("/reader?docId=source-id&page=3");
  expect(readerWitnessURL("source-id", {block_index:7})).toBe("/reader?docId=source-id&block=7");
 });
});
