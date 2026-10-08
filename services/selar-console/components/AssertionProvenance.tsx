import Link from "next/link";

export interface AssertionProvenanceLink {
  assertion_scope?: "own_work" | "reported_about_other";
  asserting_document_title?: string;
  source_document_id?: string;
  source_quote?: string;
  review_revision?: number;
}

/** Says who asserted a research relation and in what scope (issue #9). */
export function AssertionProvenance({ link }: { link: AssertionProvenanceLink }) {
  const reported = link.assertion_scope === "reported_about_other";
  return (
    <div className="assertion-provenance" data-scope={link.assertion_scope} onClick={(event) => event.stopPropagation()}
      style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 10, color: "var(--ink-3)" }}>
      <span style={{ fontFamily: "var(--font-mono)", fontSize: 9, fontWeight: 600 }}>
        Asserted by {link.asserting_document_title || "a source document"} ·{" "}
        {reported ? "reported about another work, not that work's own paper" : "the document's own work"}
        {link.review_revision ? ` · reviewed, revision ${link.review_revision}` : ""}
      </span>
      {link.source_quote && <blockquote style={{ margin: 0 }}>{link.source_quote}</blockquote>}
      {link.source_document_id && (
        <Link href={`/reader?docId=${encodeURIComponent(link.source_document_id)}`}>Open asserting document</Link>
      )}
    </div>
  );
}

