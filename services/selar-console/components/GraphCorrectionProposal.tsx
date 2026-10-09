"use client";

import { useMemo, useState } from "react";
import { clientFetch, type ChatCitation, type ChatMessage, type ResearchAssertion } from "@/lib/api";

export const RESEARCH_PREDICATES: Record<string, string> = {
  introduces: "introduces",
  uses_benchmark: "uses benchmark",
  evaluated_on: "is evaluated on",
  evaluates: "evaluates",
  compared_against: "is compared against",
  compares: "compares",
  reimplemented_as: "is re-implemented as",
  reports_result_for: "reports a result for",
  cites: "cites",
};

/**
 * Learner-authored graph correction (issue #9). The learner states the claim
 * and picks the exact passage that supports it; SELAR infers nothing. The
 * result is stored as a *proposed* assertion and changes the graph only after
 * an explicit confirmation of the exact preview shown here.
 */
export function GraphCorrectionProposal({ messages, sourceMessageId }: { messages: ChatMessage[]; sourceMessageId: string }) {
  const citations = useMemo(() => {
    const seen = new Set<string>();
    const out: ChatCitation[] = [];
    for (const message of messages) {
      if (message.role !== "assistant" || message.status === "superseded") continue;
      for (const citation of message.citations) {
        if (!seen.has(citation.chunk_id)) { seen.add(citation.chunk_id); out.push(citation); }
      }
    }
    return out;
  }, [messages]);
  const [subject, setSubject] = useState("");
  const [predicate, setPredicate] = useState("evaluated_on");
  const [object, setObject] = useState("");
  const [scope, setScope] = useState<"own_work" | "reported_about_other">("reported_about_other");
  const [context, setContext] = useState("");
  const [chunkId, setChunkId] = useState("");
  const [quote, setQuote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [proposal, setProposal] = useState<ResearchAssertion | null>(null);

  if (citations.length === 0) {
    return (
      <div className="graph-correction" role="note">
        <strong>No cited passage to support a correction yet.</strong>
        <span>Ask a question that cites the paper making the claim, then propose the correction from that passage.</span>
      </div>
    );
  }
  const citation = citations.find((item) => item.chunk_id === chunkId);

  async function propose() {
    if (!citation) return;
    setBusy(true); setError("");
    try {
      setProposal(await clientFetch<ResearchAssertion>("/api/research-assertions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject, predicate, object, scope, experiment_context: context,
          asserting_document_id: citation.document_id, source_message_id: sourceMessageId,
          evidence: [{ chunk_id: citation.chunk_id, quote }],
        }),
      }));
    } catch (proposeError) {
      setError(proposeError instanceof Error ? proposeError.message : "Unable to propose correction");
    } finally { setBusy(false); }
  }

  async function respond(action: "confirm" | "reject") {
    if (!proposal) return;
    setBusy(true); setError("");
    try {
      setProposal(await clientFetch<ResearchAssertion>(`/api/research-assertions/${proposal.id}/respond`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, revision: proposal.revision }),
      }));
    } catch (respondError) {
      setError(respondError instanceof Error ? respondError.message : "Unable to record review");
    } finally { setBusy(false); }
  }

  if (proposal) {
    return (
      <div className="graph-correction" aria-label="Proposed graph correction">
        <strong>{proposal.state === "proposed" ? "Proposed correction — not yet in your graph" : `Correction ${proposal.state === "confirmed" ? "saved" : proposal.state}`}</strong>
        <span>
          <em>{proposal.asserting_document_title}</em> asserts: {proposal.subject} {RESEARCH_PREDICATES[proposal.predicate] || proposal.predicate} {proposal.object}
          {" "}({proposal.scope === "reported_about_other" ? "reported about another work, not that work's own paper" : "its own work"})
        </span>
        {proposal.evidence.map((evidence) => <blockquote key={evidence.chunk_id}>{evidence.quote}</blockquote>)}
        {proposal.state === "proposed" && (
          <div className="graph-correction-actions">
            <button disabled={busy} onClick={() => respond("confirm")}>Save sourced assertion</button>
            <button disabled={busy} onClick={() => respond("reject")}>Discard</button>
          </div>
        )}
        {error && <span className="chat-error">{error}</span>}
      </div>
    );
  }

  const ready = subject.trim() && object.trim() && citation && quote.trim();
  return (
    <form className="graph-correction" aria-label="Propose a graph correction" onSubmit={(event) => { event.preventDefault(); if (ready) propose(); }}>
      <strong>Propose a source-scoped correction</strong>
      <span>State what one paper claims and quote the passage that says so. Save the source-scoped assertion to record your edit.</span>
      <label>Supporting passage
        <select value={chunkId} onChange={(event) => {
          setChunkId(event.target.value);
          setQuote(citations.find((item) => item.chunk_id === event.target.value)?.quote || "");
        }}>
          <option value="">Choose a cited passage…</option>
          {citations.map((item) => (
            <option key={item.chunk_id} value={item.chunk_id}>
              {item.document_title}{item.source_type === "pdf" ? `, p. ${item.page}` : ""}: {item.quote.slice(0, 80)}
            </option>
          ))}
        </select>
      </label>
      {citation && <span>Asserting document: <em>{citation.document_title}</em></span>}
      <label>Exact quote (trim to the sentence that makes the claim)
        <textarea value={quote} onChange={(event) => setQuote(event.target.value)} rows={2} maxLength={1000} />
      </label>
      <div className="graph-correction-triple">
        <input aria-label="Subject" placeholder="Subject, e.g. modified SagaLLM" value={subject} maxLength={200} onChange={(event) => setSubject(event.target.value)} />
        <select aria-label="Relation" value={predicate} onChange={(event) => setPredicate(event.target.value)}>
          {Object.entries(RESEARCH_PREDICATES).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select>
        <input aria-label="Object" placeholder="Object, e.g. τ²-bench" value={object} maxLength={200} onChange={(event) => setObject(event.target.value)} />
      </div>
      <label>Whose work is this about?
        <select value={scope} onChange={(event) => setScope(event.target.value as typeof scope)}>
          <option value="reported_about_other">Another work this paper reports on (e.g. a re-implemented baseline)</option>
          <option value="own_work">This paper&apos;s own system or experiment</option>
        </select>
      </label>
      <input aria-label="Experiment context" placeholder="Optional context, e.g. comparison baseline" value={context} maxLength={500} onChange={(event) => setContext(event.target.value)} />
      <button type="submit" disabled={!ready || busy}>Create proposal (not added to graph yet)</button>
      {error && <span className="chat-error">{error}</span>}
    </form>
  );
}
