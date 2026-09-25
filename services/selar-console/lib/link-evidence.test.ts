import { describe, expect, it } from "vitest";
import { groundedLinkPresentation } from "./link-evidence";

const link = {
  source_document_id: "new", target_document_id: "prior",
  source_document_title: "New reading", target_document_title: "Prior reading",
  source_quote: "The new passage asserts gradient descent optimization.",
  target_quote: "The prior passage discusses gradient descent optimization.",
  source_locator: { page: 2 }, target_locator: { page: 4 },
};

describe("learner evidence pair", () => {
  it("shows both exact sources in new → prior order with locators", () => {
    expect(groundedLinkPresentation(link)).toEqual({
      evidence: "New reading · p.2: “The new passage asserts gradient descent optimization.”\nPrior reading · p.4: “The prior passage discusses gradient descent optimization.”",
      prompt: "How do these passages treat the shared concept? Compare their claims using both sources.",
    });
  });
  it("abstains when either quote or locator is missing", () => {
    expect(groundedLinkPresentation({ ...link, target_quote: "" })).toBeNull();
    expect(groundedLinkPresentation({ ...link, source_locator: {} })).toBeNull();
  });
});
