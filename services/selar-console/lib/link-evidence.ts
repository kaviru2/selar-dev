type EvidenceLink = {
  source_document_title: string;
  target_document_title: string;
  source_quote?: string;
  target_quote?: string;
  source_locator?: Record<string, unknown>;
  target_locator?: Record<string, unknown>;
};

function location(locator?: Record<string, unknown>): string | null {
  if (!locator) return null;
  if (typeof locator.page === "number" && locator.page > 0) return `p.${locator.page}`;
  if (typeof locator.block_index === "number") return `block ${locator.block_index}`;
  return null;
}

/** Only the server's verified two-source witnesses can form a link card. */
export function groundedLinkPresentation(link: EvidenceLink) {
  const source = location(link.source_locator);
  const target = location(link.target_locator);
  if (!source || !target || !link.source_quote || !link.target_quote ||
      !link.source_document_title || !link.target_document_title) return null;
  return {
    evidence: `${link.source_document_title} · ${source}: “${link.source_quote}”\n${link.target_document_title} · ${target}: “${link.target_quote}”`,
    prompt: "How do these passages treat the shared concept? Compare their claims using both sources.",
  };
}
