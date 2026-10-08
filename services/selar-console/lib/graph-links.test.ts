import { describe, expect, it } from "vitest";
import {
  candidateReviewURL,
  crossReadingNotice,
  crossReadingSummary,
  isCandidateLink,
  linkDash,
  linkLayer,
} from "./graph-links";

const docs = ["model:a", "model:b", "model:c", "model:d", "model:e"].map((id) => ({ id, node_type: "document" as const }));
const structure = [
  { source: "model:a", target: "claim:a", relation: "has_claim", state: "confirmed", created_via: "system" },
  { source: "model:a", target: "concept-1", relation: "uses_concept", state: "supported", created_via: "system" },
];
const reviewed = { source: "model:d", target: "model:e", state: "confirmed", created_via: "user_reviewed" };
const candidate = { source: "model:c", target: "model:a", state: "candidate", created_via: "ai_suggested", candidate_link_id: "L1" };

describe("graph link presentation (#117)", () => {
  it("separates machine candidates from reviewed links and PDF structure", () => {
    expect(isCandidateLink(candidate)).toBe(true);
    expect(linkLayer(candidate)).toBe("candidate");
    expect(linkLayer(reviewed)).toBe("yours");
    expect(linkLayer(structure[0])).toBe("pdf");
    // A legacy chat edge in "candidate" state is still the learner's layer, not a SELAR suggestion.
    expect(linkLayer({ state: "candidate", created_via: "deterministic_chat" })).toBe("yours");
  });

  it("draws confirmed links solid and candidates dashed", () => {
    expect(linkDash(reviewed)).toEqual([]);
    expect(linkDash(candidate).length).toBe(2);
    expect(linkDash({ state: "rejected", created_via: "user_reviewed" }).length).toBe(2);
  });

  it("reproduces the reported state: one reviewed link, three unconnected readings", () => {
    const summary = crossReadingSummary(docs, [...structure, reviewed]);
    expect(summary).toEqual({ readings: 5, reviewed: 1, candidates: 0, unconnected: 3 });
    const notice = crossReadingNotice(summary, true);
    expect(notice?.title).toBe("3 readings are not linked to the others");
    expect(notice?.body).toMatch(/both name the same concept/);
    expect(notice?.body).toMatch(/not links/);
  });

  it("explains an empty library honestly and never blames processing", () => {
    const notice = crossReadingNotice(crossReadingSummary(docs, structure), true);
    expect(notice?.title).toBe("Your readings are not linked yet");
    expect(notice?.body).not.toMatch(/at least two/);
    expect(crossReadingNotice(crossReadingSummary(docs.slice(0, 1), []), true)).toBeNull();
  });

  it("asks for review when candidates exist, and says when they are hidden", () => {
    const summary = crossReadingSummary(docs, [...structure, reviewed, candidate]);
    expect(summary.candidates).toBe(1);
    expect(summary.unconnected).toBe(1);
    expect(crossReadingNotice(summary, true)?.title).toBe("1 suggested link to review");
    expect(crossReadingNotice(summary, true)?.body).toMatch(/Nothing is added/);
    expect(crossReadingNotice(summary, false)?.body).toMatch(/Turn on “To review”/);
  });

  it("ignores inactive cross-reading links when counting connections", () => {
    const summary = crossReadingSummary(docs, [{ ...reviewed, state: "rejected" }]);
    expect(summary.reviewed).toBe(0);
    expect(summary.unconnected).toBe(5);
  });

  it("links a candidate to its guided review in the Reader", () => {
    expect(candidateReviewURL({ source_document_id: "doc 1", candidate_link_id: "L1" })).toBe("/reader?docId=doc+1&linkId=L1");
    expect(candidateReviewURL({ source_document_id: "doc", candidate_link_id: undefined })).toBeNull();
  });
});
