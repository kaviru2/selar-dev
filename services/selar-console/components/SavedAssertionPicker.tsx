"use client";

import { useState } from "react";
import { clientFetch, type ResearchAssertion } from "@/lib/api";

export interface AssertionSelection {
  asserting_document_id: string;
  assertion_id: string;
  revision: number;
}

export function savedAssertionQuestion(a: ResearchAssertion): string {
  return "Show saved assertion: " + JSON.stringify([a.subject, a.predicate, a.object, a.scope,
    a.experiment_context || "", a.subject_qualifier || "", a.object_qualifier || ""]);
}

export function SavedAssertionPicker({ onSelect, disabled }: {
  onSelect: (selection: AssertionSelection | undefined, question?: string) => void;
  disabled: boolean;
}) {
  const [items, setItems] = useState<ResearchAssertion[]>([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [document, setDocument] = useState("");
  const [assertion, setAssertion] = useState("");
  async function load() {
    setOpen(true); setLoading(true); setError("");
    setDocument(""); setAssertion(""); onSelect(undefined, "");
    try {
      const rows = await clientFetch<ResearchAssertion[]>("/api/research-assertions?state=confirmed");
      setItems(rows.filter(a => a.state === "confirmed" && !a.superseded_by && a.evidence.length > 0));
    } catch (e) { setItems([]); setError(e instanceof Error ? e.message : "Unable to load assertions"); }
    finally { setLoading(false); }
  }
  const documents = Array.from(new Map(items.map(a => [a.asserting_document_id, a.asserting_document_title])));
  const selected = items.find(a => a.id === assertion && a.asserting_document_id === document);
  return <section aria-label="Saved assertion display">
    <button type="button" disabled={disabled || loading} onClick={load}>Browse saved assertions</button>
    {open && <>
      <p>Read-only saved-claim display, not proof of entailment or original-source identity. Select the asserting document, then an already confirmed assertion. No extraction or graph changes.</p>
      {error && <p role="alert">{error}</p>}
      {loading ? <p>Loading saved assertions…</p> : <>
        <label>Asserting document <select aria-label="Asserting document" value={document} disabled={disabled}
          onChange={e => { setDocument(e.target.value); setAssertion(""); onSelect(undefined, ""); }}>
          <option value="">Select a document</option>
          {documents.map(([id,title]) => <option key={id} value={id}>{title}</option>)}
        </select></label>
        <label>Confirmed saved assertion <select aria-label="Confirmed saved assertion" value={assertion} disabled={disabled || !document}
          onChange={e => {
            setAssertion(e.target.value);
            const a = items.find(a => a.id === e.target.value && a.asserting_document_id === document);
            onSelect(a ? {asserting_document_id:document, assertion_id:a.id, revision:a.revision} : undefined,
              a ? savedAssertionQuestion(a) : "");
          }}>
          <option value="">Select an assertion</option>
          {items.filter(a => a.asserting_document_id === document).map(a =>
            <option key={a.id} value={a.id}>{a.subject} — {a.predicate} — {a.object} ({a.scope})</option>)}
        </select></label>
        {!items.length && <p>No confirmed assertions with live witnesses are available.</p>}
        {selected && <div>
          <p>Scope: {selected.scope}. Subject variant: {selected.subject_qualifier || "unspecified"}. Object variant: {selected.object_qualifier || "unspecified"}. Context: {selected.experiment_context || "unspecified"}. Revision: {selected.revision}.</p>
          {selected.evidence.map(e => <blockquote key={e.chunk_id}>{e.quote}</blockquote>)}
          <p>Ask sends the exact saved-claim request below. Edited factual questions abstain in this mode; quote membership is not claim verification.</p>
        </div>}
      </>}
      <button type="button" disabled={disabled || loading} onClick={() => {setOpen(false);setDocument("");setAssertion("");onSelect(undefined, "");}}>Return to ordinary chat</button>
    </>}
  </section>;
}
