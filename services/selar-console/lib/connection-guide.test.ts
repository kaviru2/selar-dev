import { describe, expect, it } from "vitest";
import type { MentalModelLink } from "./api";
import { keptLinks, linksToReview, locatorLabel, otherSide, overlapStrength, setAsideLinks, sharedIdea, whySuggested } from "./connection-guide";

const base = {
  id: "a", user_id: "u", source_model_id: "m1", target_model_id: "m2",
  source_document_id: "new", target_document_id: "old",
  source_document_title: "Week 5 PDF", target_document_title: "Week 3 notes",
  link_type: "concept_overlap", similarity: 0.7, confidence: 0.65,
  bridge_explanation: "Both readings explicitly discuss Retrieval Practice. Compare their treatment.",
  status: "candidate", created_via: "ai_suggested", suggested_at: "2026-10-01T00:00:00Z", revision: 0,
} as MentalModelLink;

describe("connection guide helpers", () => {
  it("splits links into to-review, kept and set-aside", () => {
    const links = [
      { ...base, id: "c2", suggested_at: "2026-10-02T00:00:00Z" },
      { ...base, id: "c1" },
      { ...base, id: "k", status: "confirmed" },
      { ...base, id: "r", status: "relabeled" },
      { ...base, id: "x", status: "rejected" },
      { ...base, id: "z", status: "archived" },
    ] as MentalModelLink[];
    expect(linksToReview(links).map((l) => l.id)).toEqual(["c1", "c2"]);
    expect(keptLinks(links).map((l) => l.id).sort()).toEqual(["k", "r"]);
    expect(setAsideLinks(links).map((l) => l.id).sort()).toEqual(["x", "z"]);
  });
  it("names the other document relative to the open reading", () => {
    expect(otherSide(base, "new").otherTitle).toBe("Week 3 notes");
    expect(otherSide(base, "old").otherTitle).toBe("Week 5 PDF");
    expect(otherSide(base, "old").otherId).toBe("new");
  });
  it("uses words, not percentages, for strength", () => {
    expect(overlapStrength(0.3)).toBe("Weak overlap");
    expect(overlapStrength(0.65)).toBe("Moderate overlap");
    expect(overlapStrength(0.9)).toBe("Strong overlap");
  });
  it("extracts the shared idea and degrades gracefully", () => {
    expect(sharedIdea(base)).toBe("Retrieval Practice");
    expect(whySuggested(base)).toBe("Both readings discuss retrieval practice.");
    expect(sharedIdea({ bridge_explanation: "Something else entirely" })).toBeNull();
    expect(whySuggested({ bridge_explanation: "" })).toBe("Both readings share a concept.");
  });
  it("formats locators", () => {
    expect(locatorLabel({ page: 3 })).toBe("p. 3");
    expect(locatorLabel({ block_index: 0 })).toBe("section 1");
    expect(locatorLabel(undefined)).toBe("");
  });
});
