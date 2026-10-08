// Plain-language presentation helpers for the guided Connections sidebar.
// These never change evidence or review semantics; they only translate the
// server's grounded link fields into learner-facing words.
import type { MentalModelLink } from "./api";

export type GuideStep = "notice" | "explain" | "compare";

/** Links that still need a learner decision, oldest suggestion first. */
export function linksToReview(links: MentalModelLink[]): MentalModelLink[] {
  return links
    .filter((link) => link.status === "candidate")
    .sort((a, b) => (a.suggested_at || "").localeCompare(b.suggested_at || ""));
}

/** Links the learner has kept (confirmed, or kept with a different relationship note). */
export function keptLinks(links: MentalModelLink[]): MentalModelLink[] {
  return links
    .filter((link) => link.status === "confirmed" || link.status === "relabeled")
    .sort((a, b) => (b.responded_at || "").localeCompare(a.responded_at || ""));
}

/** Links the learner has said are not real (rejected or later removed). */
export function setAsideLinks(links: MentalModelLink[]): MentalModelLink[] {
  return links.filter((link) => link.status === "rejected" || link.status === "archived");
}

/** The document on the other side of a link, relative to the open reading. */
export function otherSide(link: MentalModelLink, docId: string) {
  const isSource = link.source_document_id === docId;
  return {
    thisTitle: isSource ? link.source_document_title : link.target_document_title,
    otherTitle: isSource ? link.target_document_title : link.source_document_title,
    otherId: isSource ? link.target_document_id : link.source_document_id,
  };
}

/** Strength in words instead of a percentage, so a score is not read as proof. */
export function overlapStrength(confidence: number): "Weak overlap" | "Moderate overlap" | "Strong overlap" {
  if (confidence >= 0.8) return "Strong overlap";
  if (confidence >= 0.55) return "Moderate overlap";
  return "Weak overlap";
}

/**
 * The shared idea named in the server's bridge explanation, e.g.
 * "Both readings explicitly discuss Retrieval Practice. Compare…" → "Retrieval Practice".
 */
export function sharedIdea(link: Pick<MentalModelLink, "bridge_explanation">): string | null {
  const match = /discuss\s+(.+?)\.(\s|$)/i.exec(link.bridge_explanation || "");
  const idea = match?.[1]?.trim();
  return idea && idea.length <= 80 ? idea : null;
}

/** One-sentence reason SELAR suggests the link, without overstating it. */
export function whySuggested(link: Pick<MentalModelLink, "bridge_explanation">): string {
  const idea = sharedIdea(link);
  return idea
    ? `Both readings discuss ${idea.toLowerCase()}.`
    : "Both readings share a concept.";
}

export function locatorLabel(locator?: { page?: number; block_index?: number }): string {
  if (locator?.page) return `p. ${locator.page}`;
  if (locator?.block_index !== undefined) return `section ${locator.block_index + 1}`;
  return "";
}

/** Learner-facing words for the stored review history actions. */
export const DECISION_WORDS: Record<string, string> = {
  confirmed: "Kept the link",
  relabeled: "Kept with a different relationship",
  rejected: "Marked as not a real link",
  retracted: "Removed a kept link",
  rolled_back: "Undid the last decision",
};
