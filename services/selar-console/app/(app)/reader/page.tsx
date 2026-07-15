"use client";

import dynamic from "next/dynamic";
import { useSearchParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { Icon } from "@/components/ui/Icon";
import {
  clientFetch,
  type Annotation,
  type DocumentMentalModel,
  type LinkSuggestion,
  type MentalModelLink,
} from "@/lib/api";

const PdfCanvas = dynamic(() => import("@/components/PdfCanvas"), {
  ssr: false,
  loading: () => (
    <div className="reader-empty">Initializing PDF engine…</div>
  ),
});

const RELATION_LABELS: Record<string, string> = {
  related_to: "related to",
  prerequisite_of: "prerequisite of",
  sub_concept_of: "sub-concept of",
  contradicts: "contradicts",
  extends: "extends",
  concept_overlap: "concept overlap",
  claim_extension: "claim extension",
  assumption_conflict: "assumption conflict",
  question_resolution: "question resolution",
};

const RELATION_COLORS: Record<string, string> = {
  prerequisite_of: "var(--accent)",
  extends: "var(--accent-2)",
  sub_concept_of: "var(--accent-3)",
  contradicts: "#c0443a",
  related_to: "var(--ink-4)",
  concept_overlap: "#5e8aaa",
  claim_extension: "var(--accent-2)",
  assumption_conflict: "#c0443a",
  question_resolution: "#8a6a9a",
};

type PanelMode = "argument" | "passages";

export default function ReaderPage() {
  const searchParams = useSearchParams();
  const [docId, setDocId] = useState(searchParams.get("docId") || "");
  const [suggestions, setSuggestions] = useState<LinkSuggestion[]>([]);
  const [mentalLinks, setMentalLinks] = useState<MentalModelLink[]>([]);
  const [mentalModel, setMentalModel] = useState<DocumentMentalModel | null>(null);
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [loading, setLoading] = useState(Boolean(searchParams.get("docId")));
  const [panelMode, setPanelMode] = useState<PanelMode>("argument");
  const [zoom, setZoom] = useState(1);
  const [annotationsOn, setAnnotationsOn] = useState(true);
  const [numPages, setNumPages] = useState(0);
  const [pageNumber, setPageNumber] = useState(1);

  useEffect(() => {
    if (!docId) return;
    let cancelled = false;
    Promise.all([
      clientFetch<LinkSuggestion[]>(`/api/documents/${docId}/suggestions?page=0`).catch(() => []),
      clientFetch<Annotation[]>(`/api/documents/${docId}/annotations`).catch(() => []),
      clientFetch<DocumentMentalModel>(`/api/documents/${docId}/mental-model`).catch(() => null),
      clientFetch<MentalModelLink[]>(`/api/mental-model-links?document_id=${docId}`).catch(() => []),
    ]).then(([suggestionData, annotationData, modelData, linkData]) => {
      if (cancelled) return;
      setSuggestions(suggestionData);
      setAnnotations(annotationData);
      setMentalModel(modelData);
      setMentalLinks(linkData);
      setLoading(false);
    });

    return () => {
      cancelled = true;
    };
  }, [docId]);

  const selectDocument = useCallback((id: string) => {
    setLoading(true);
    setPageNumber(1);
    setDocId(id);
  }, []);

  const pendingCount = panelMode === "argument"
    ? mentalLinks.filter((item) => item.status === "candidate").length
    : suggestions.filter((item) => item.status === "pending").length;
  const confirmedCount = panelMode === "argument"
    ? mentalLinks.filter((item) => item.status === "confirmed").length
    : suggestions.filter((item) => item.status === "confirmed").length;

  async function createAnnotation(
    type: string,
    bboxes: Array<{ x: number; y: number; w: number; h: number }>,
    color: string,
    page: number,
    comment = ""
  ) {
    const annotation = await clientFetch<Annotation>("/api/annotations", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        document_id: docId,
        page,
        bbox: bboxes,
        type,
        color,
        comment,
      }),
    });
    setAnnotations((current) => [...current, annotation]);
  }

  async function respondToPassage(id: string, action: "confirmed" | "rejected") {
    const previous = suggestions;
    setSuggestions((current) => current.map((item) => item.id === id ? { ...item, status: action } : item));
    try {
      await clientFetch(`/api/suggestions/${id}/respond`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, time_to_respond_ms: 1500 }),
      });
    } catch (error) {
      setSuggestions(previous);
      console.error("Failed to save passage response", error);
    }
  }

  async function respondToMentalLink(id: string, action: "confirmed" | "rejected") {
    const previous = mentalLinks;
    setMentalLinks((current) => current.map((item) => item.id === id ? { ...item, status: action } : item));
    try {
      await clientFetch(`/api/mental-model-links/${id}/respond`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action }),
      });
    } catch (error) {
      setMentalLinks(previous);
      console.error("Failed to save mental-model response", error);
    }
  }

  return (
    <div className="reader">
      <Sidebar currentId={docId} onPick={selectDocument} />

      <main className="doc-pane">
        <div className="doc-toolbar">
          <div className="grp">
            <button aria-label="Previous page" disabled={pageNumber <= 1} onClick={() => setPageNumber((page) => Math.max(1, page - 1))}>‹</button>
            <span className="page-indicator">{pageNumber} / {numPages || "?"}</span>
            <button aria-label="Next page" disabled={!numPages || pageNumber >= numPages} onClick={() => setPageNumber((page) => Math.min(numPages, page + 1))}>›</button>
          </div>
          <div className="grp">
            <button aria-label="Zoom out" onClick={() => setZoom((value) => Math.max(0.6, value - 0.1))}><Icon name="zoom_out" size={12} /></button>
            <span className="page-indicator">{Math.round(zoom * 100)}%</span>
            <button aria-label="Zoom in" onClick={() => setZoom((value) => Math.min(1.8, value + 0.1))}><Icon name="zoom_in" size={12} /></button>
          </div>
          <div className="grp">
            <button className={annotationsOn ? "on" : ""} onClick={() => setAnnotationsOn((value) => !value)}>
              <Icon name="highlight" size={12} /> Marks
            </button>
          </div>
          <div className="tool-spacer" />
          {mentalModel && <span className="mental-domain-chip">{mentalModel.domain || "Mental model ready"}</span>}
        </div>

        <div className="pdf-container">
          {docId ? (
            <PdfCanvas
              docId={docId}
              zoom={zoom}
              pageNumber={pageNumber}
              annotationsOn={annotationsOn}
              suggestions={panelMode === "passages" ? suggestions : []}
              annotations={annotations}
              onCreateAnnotation={createAnnotation}
              onPageLoad={setNumPages}
            />
          ) : (
            <div className="reader-empty">
              <Icon name="book" size={24} />
              <strong>Select a document to begin reading</strong>
              <span>SELAR will surface evidence-backed connections here.</span>
            </div>
          )}
        </div>
      </main>

      <aside className="matches mental-panel">
        <div className="matches-head">
          <span className="ttl">Connections</span>
          <span className="chip">{pendingCount} to review</span>
          <span className="chip linked-chip">{confirmedCount} linked</span>
        </div>

        <div className="mental-panel-tabs" role="tablist" aria-label="Connection type">
          <button role="tab" aria-selected={panelMode === "argument"} className={panelMode === "argument" ? "active" : ""} onClick={() => setPanelMode("argument")}>Argument links</button>
          <button role="tab" aria-selected={panelMode === "passages"} className={panelMode === "passages" ? "active" : ""} onClick={() => setPanelMode("passages")}>Passage matches</button>
        </div>

        <div className="matches-body">
          {panelMode === "argument" ? (
            <>
              {mentalModel && <MentalModelSummary model={mentalModel} />}
              <div className="match-group-lbl">Argument-level candidates · {loading ? "…" : mentalLinks.length}</div>
              {!loading && mentalLinks.length === 0 && <PanelEmpty message="No argument-level links yet. They appear after at least two documents have mental models." />}
              {mentalLinks.map((link) => {
                const otherTitle = link.source_document_id === docId ? link.target_document_title : link.source_document_title;
                return (
                  <ConnectionCard
                    key={link.id}
                    relation={link.link_type}
                    score={link.confidence}
                    source={otherTitle}
                    explanation={link.bridge_explanation}
                    evidence={link.target_evidence || link.source_evidence}
                    status={link.status}
                    onConfirm={() => respondToMentalLink(link.id, "confirmed")}
                    onReject={() => respondToMentalLink(link.id, "rejected")}
                  />
                );
              })}
            </>
          ) : (
            <>
              <div className="match-group-lbl">Passage-level matches · {loading ? "…" : suggestions.length}</div>
              {!loading && suggestions.length === 0 && <PanelEmpty message="No passage matches were generated for this document." />}
              {suggestions.map((suggestion) => (
                <ConnectionCard
                  key={suggestion.id}
                  relation={suggestion.relation}
                  score={suggestion.similarity}
                  source={`${suggestion.tgt_doc || "Unknown source"} · p.${suggestion.tgt_page}`}
                  explanation={suggestion.summary || suggestion.tgt_text}
                  status={suggestion.status}
                  onConfirm={() => respondToPassage(suggestion.id, "confirmed")}
                  onReject={() => respondToPassage(suggestion.id, "rejected")}
                />
              ))}
            </>
          )}
        </div>
      </aside>
    </div>
  );
}

function MentalModelSummary({ model }: { model: DocumentMentalModel }) {
  return (
    <section className="mental-summary">
      <div className="mental-summary-kicker"><Icon name="graph" size={11} /> Article mental model</div>
      <p>{model.main_claim}</p>
      <div className="mental-concepts">
        {model.key_concepts.slice(0, 6).map((concept) => <span key={concept}>{concept}</span>)}
      </div>
      {(model.assumptions.length > 0 || model.open_questions.length > 0) && (
        <div className="mental-summary-meta">
          <span>{model.assumptions.length} assumptions</span>
          <span>{model.open_questions.length} open questions</span>
        </div>
      )}
    </section>
  );
}

function ConnectionCard({ relation, score, source, explanation, evidence, status, onConfirm, onReject }: {
  relation: string;
  score: number;
  source: string;
  explanation: string;
  evidence?: string;
  status: string;
  onConfirm: () => void;
  onReject: () => void;
}) {
  const relationColor = RELATION_COLORS[relation] || "var(--ink-4)";
  const reviewed = status === "confirmed" || status === "rejected" || status === "relabeled";
  return (
    <article className={`match-card mental-link-card ${status}`}>
      <div className="row1">
        <span className="relation-pill" style={{ color: relationColor, borderColor: relationColor }}>{RELATION_LABELS[relation] || relation.replaceAll("_", " ")}</span>
        <span className="sim">{Math.round(score * 100)}%</span>
        <div className="sim-bar"><div className="fill" style={{ width: `${Math.max(0, Math.min(100, score * 100))}%` }} /></div>
      </div>
      <div className="connection-source">{source}</div>
      <p className="connection-explanation">{explanation}</p>
      {evidence && <details className="connection-evidence"><summary>View supporting passage</summary><p>{evidence}</p></details>}
      <div className="foot">
        {reviewed ? (
          <span className={`review-state ${status}`}><Icon name={status === "rejected" ? "x" : "check"} size={11} /> {status}</span>
        ) : (
          <>
            <button className="confirm" onClick={onConfirm}><Icon name="check" size={11} /> Confirm</button>
            <button onClick={onReject}><Icon name="x" size={11} /> Reject</button>
          </>
        )}
      </div>
    </article>
  );
}

function PanelEmpty({ message }: { message: string }) {
  return <div className="panel-empty"><Icon name="link" size={18} /><span>{message}</span></div>;
}
