"use client";

import dynamic from "next/dynamic";
import { useSearchParams } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { Icon } from "@/components/ui/Icon";
import { ArticleReader } from "@/components/ArticleReader";
import { createReaderTelemetry, type ReaderTelemetry } from "@/lib/reader-telemetry";
import { groundedLinkPresentation } from "@/lib/link-evidence";
import { ReviewAssertion } from "@/components/ReviewAssertion";
import { type ReviewAction } from "@/lib/reviewed-links";
import {
  clientFetch,
  type Annotation,
  type DocumentContent,
  type DocumentMentalModel,
  type LinkSuggestion,
  type MentalModelLink,
  type MentalLinkReviewPreview,
} from "@/lib/api";

const PdfCanvas = dynamic(() => import("@/components/PdfCanvas"), {
  ssr: false,
  loading: () => (
    <div className="reader-empty">Initializing PDF engine…</div>
  ),
});

const RELATION_LABELS: Record<string, string> = {
  unclassified: "unclassified passage match",
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
  const [reviewPreview, setReviewPreview] = useState<MentalLinkReviewPreview | null>(null);
  const [reviewLabel, setReviewLabel] = useState("");
  const [reviewReason, setReviewReason] = useState("");
  const [reviewBusy, setReviewBusy] = useState(false);
  const [reviewError, setReviewError] = useState("");
  const [mentalModel, setMentalModel] = useState<DocumentMentalModel | null>(null);
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [documentContent, setDocumentContent] = useState<DocumentContent | null>(null);
  const [loading, setLoading] = useState(Boolean(searchParams.get("docId")));
  const [suggestionError, setSuggestionError] = useState("");
  const [suggestionActionError, setSuggestionActionError] = useState("");
  const [panelMode, setPanelMode] = useState<PanelMode>("argument");
  const [zoom, setZoom] = useState(1);
  const [annotationsOn, setAnnotationsOn] = useState(true);
  const [suggestionsOn, setSuggestionsOn] = useState(true);
  const [numPages, setNumPages] = useState(0);
  const [pageNumber, setPageNumber] = useState(() => {
    const requestedPage = Number(searchParams.get("page") || "1");
    return Number.isFinite(requestedPage) ? Math.max(1, requestedPage) : 1;
  });
  const telemetryRef = useRef<ReaderTelemetry | null>(null);

  useEffect(() => {
    if (!docId) return;
    let cancelled = false;
    Promise.all([
      clientFetch<LinkSuggestion[]>(`/api/documents/${docId}/suggestions?page=0`).catch((error) => {
        if (!cancelled) setSuggestionError(error instanceof Error ? error.message : "Unable to load suggested passages");
        return [];
      }),
      clientFetch<Annotation[]>(`/api/documents/${docId}/annotations`).catch(() => []),
      clientFetch<DocumentMentalModel>(`/api/documents/${docId}/mental-model`).catch(() => null),
      clientFetch<MentalModelLink[]>(`/api/mental-model-links?document_id=${docId}`).catch(() => []),
      clientFetch<DocumentContent>(`/api/documents/${docId}/content`).catch(() => null),
    ]).then(([suggestionData, annotationData, modelData, linkData, contentData]) => {
      if (cancelled) return;
      setSuggestions(suggestionData);
      setAnnotations(annotationData);
      setMentalModel(modelData);
      setMentalLinks(linkData);
      setDocumentContent(contentData);
      setLoading(false);
    });

    return () => {
      cancelled = true;
    };
  }, [docId]);

  useEffect(() => {
    const linkId = searchParams.get("linkId");
    if (!linkId || !mentalLinks.some(link => link.id === linkId)) return;
    let active = true;
    clientFetch<MentalLinkReviewPreview>(`/api/mental-model-links/${linkId}/preview`)
      .then(preview => { if (active) { setReviewPreview(preview); setReviewLabel(preview.user_label || ""); } })
      .catch(error => { if (active) setReviewError(error instanceof Error ? error.message : "Assertion unavailable"); });
    return () => { active = false; };
  }, [mentalLinks, searchParams]);

  useEffect(() => {
    if (!docId || documentContent?.document.status !== "ready") return;

    const telemetry = createReaderTelemetry();
    telemetryRef.current = telemetry;
    let closed = false;
    let sessionID = "";

    const endSession = (id: string) => {
      void clientFetch(`/api/sessions/${id}/end`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(telemetry.sessionSummary()),
      }).catch(() => undefined);
    };

    void clientFetch<{ id: string }>("/api/sessions/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ document_id: docId }),
    }).then((session) => {
      sessionID = session.id;
      if (closed) endSession(sessionID);
    }).catch(() => undefined);

    return () => {
      closed = true;
      if (sessionID) endSession(sessionID);
      if (telemetryRef.current === telemetry) telemetryRef.current = null;
    };
  }, [docId, documentContent?.document.status]);

  useEffect(() => {
    telemetryRef.current?.visitPage(pageNumber);
  }, [pageNumber]);

  useEffect(() => {
    for (const suggestion of suggestions) {
      if (suggestion.status === "pending") telemetryRef.current?.showSuggestion(suggestion.id);
    }
  }, [suggestions]);

  const openDocument = useCallback((id: string, page = 1) => {
    setLoading(true);
    setSuggestionError("");
    setSuggestionActionError("");
    setPageNumber(Math.max(1, page));
    setSuggestions([]);
    setMentalLinks([]);
    setMentalModel(null);
    setAnnotations([]);
    setDocumentContent(null);
    setDocId(id);
  }, []);

  const selectDocument = useCallback((id: string) => openDocument(id, 1), [openDocument]);

  const pendingCount = panelMode === "argument"
    ? mentalLinks.filter((item) => item.status === "candidate").length
    : suggestions.filter((item) => item.status === "pending").length;
  const confirmedCount = panelMode === "argument"
    ? mentalLinks.filter((item) => item.status === "confirmed" || item.status === "relabeled").length
    : suggestions.filter((item) => item.status === "confirmed").length;
  const visibleSuggestions = suggestions.filter((item) => item.status !== "rejected");
  const currentPageSuggestionCount = visibleSuggestions.filter((item) => item.src_page === pageNumber).length;
  const isPdf = !documentContent || documentContent.document.source_type === "pdf";
  const ingestionState = documentContent?.document.status;

  const goToNextSuggestion = useCallback(() => {
    const pages = Array.from(new Set(
      suggestions
        .filter((item) => item.status !== "rejected" && item.src_page > 0)
        .map((item) => item.src_page)
    )).sort((left, right) => left - right);
    if (!pages.length) return;
    setSuggestionsOn(true);
    setPageNumber(pages.find((page) => page > pageNumber) || pages[0]);
  }, [pageNumber, suggestions]);

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

  async function respondToPassage(id: string, action: "confirmed" | "rejected"): Promise<boolean> {
    const previous = suggestions;
    setSuggestionActionError("");
    setSuggestions((current) => current.map((item) => item.id === id ? { ...item, status: action } : item));
    try {
      await clientFetch(`/api/suggestions/${id}/respond`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, time_to_respond_ms: telemetryRef.current?.responseTime(id) || 0 }),
      });
      const persisted = await clientFetch<LinkSuggestion[]>(`/api/documents/${docId}/suggestions?page=0`);
      setSuggestions(persisted);
      return true;
    } catch (error) {
      setSuggestions(previous);
      setSuggestionActionError(error instanceof Error ? error.message : "Unable to save suggestion response");
      console.error("Failed to save passage response", error);
      return false;
    }
  }

  async function previewMentalLink(id: string) {
    setReviewError("");
    try {
      const preview = await clientFetch<MentalLinkReviewPreview>(`/api/mental-model-links/${id}/preview`);
      setReviewPreview(preview);
      setReviewLabel(preview.user_label || "");
      setReviewReason("");
    } catch (error) {
      setReviewPreview(null);
      setReviewError(error instanceof Error ? error.message : "Assertion preview unavailable");
    }
  }

  async function respondToMentalLink(action: ReviewAction) {
    if (!reviewPreview || reviewBusy) return;
    setReviewBusy(true);
    setReviewError("");
    try {
      await clientFetch(`/api/mental-model-links/${reviewPreview.id}/respond`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, revision: reviewPreview.revision,
          label: action === "relabeled" ? reviewLabel : "", reason: reviewReason,
          target_revision: action === "rolled_back" ? reviewPreview.revision - 1 : undefined }),
      });
      const links = await clientFetch<MentalModelLink[]>(`/api/mental-model-links?document_id=${docId}`);
      setMentalLinks(links);
      await previewMentalLink(reviewPreview.id);
    } catch (error) {
      setReviewError(error instanceof Error ? error.message : "Could not save assertion review");
      setReviewPreview(null); // never reuse a possibly stale revision
    } finally {
      setReviewBusy(false);
    }
  }

  return (
    <div className="reader">
      <Sidebar currentId={docId} onPick={selectDocument} />

      <main className="doc-pane">
        <div className="doc-toolbar">
          {isPdf && <div className="grp">
            <button aria-label="Previous page" disabled={pageNumber <= 1} onClick={() => setPageNumber((page) => Math.max(1, page - 1))}>‹</button>
            <span className="page-indicator">{pageNumber} / {numPages || "?"}</span>
            <button aria-label="Next page" disabled={!numPages || pageNumber >= numPages} onClick={() => setPageNumber((page) => Math.min(numPages, page + 1))}>›</button>
          </div>}
          {isPdf && <div className="grp">
            <button aria-label="Zoom out" onClick={() => setZoom((value) => Math.max(0.6, value - 0.1))}><Icon name="zoom_out" size={12} /></button>
            <span className="page-indicator">{Math.round(zoom * 100)}%</span>
            <button aria-label="Zoom in" onClick={() => setZoom((value) => Math.min(1.8, value + 0.1))}><Icon name="zoom_in" size={12} /></button>
          </div>}
          <div className="grp">
            <button className={annotationsOn ? "on" : ""} onClick={() => setAnnotationsOn((value) => !value)}>
              <Icon name="highlight" size={12} /> Marks
            </button>
            <button className={suggestionsOn ? "on suggestion-toggle" : "suggestion-toggle"} onClick={() => setSuggestionsOn((value) => !value)}>
              <Icon name="link" size={12} /> Suggestions {visibleSuggestions.length}
            </button>
            <button
              aria-label="Go to next suggested passage"
              disabled={visibleSuggestions.length === 0}
              onClick={goToNextSuggestion}
              title={currentPageSuggestionCount > 0 ? `${currentPageSuggestionCount} suggestion(s) on this page` : "Go to the next page with a suggestion"}
            >{currentPageSuggestionCount > 0 ? `${currentPageSuggestionCount} highlighted · Next ›` : "Next match ›"}</button>
          </div>
          <div className="tool-spacer" />
          {mentalModel && <span className="mental-domain-chip">{mentalModel.domain || "Mental model ready"}</span>}
        </div>

        <div className={isPdf ? "pdf-container" : "article-container"}>
          {loading ? (
            <div className="reader-empty">Loading source snapshot…</div>
          ) : ingestionState === "processing" ? (
            <div className="reader-empty"><Icon name="spinner" size={20} className="animate-spin" /><strong>Processing this source</strong><span>You can return to the Library while the durable ingestion job runs.</span></div>
          ) : ingestionState === "failed" ? (
            <div className="reader-empty"><Icon name="x" size={20} /><strong>Source processing failed</strong><span>{documentContent?.document.ingestion_error || "Open the Library to retry this document."}</span></div>
          ) : docId && !isPdf && documentContent ? (
            <ArticleReader content={documentContent} targetBlock={searchParams.has("block") ? Number(searchParams.get("block")) : undefined} />
          ) : docId ? (
            <PdfCanvas
              docId={docId}
              zoom={zoom}
              pageNumber={pageNumber}
              annotationsOn={annotationsOn}
              suggestionsOn={suggestionsOn}
              suggestions={suggestions}
              annotations={annotations}
              onCreateAnnotation={createAnnotation}
              onPageLoad={setNumPages}
              onScrollDepth={(depth) => telemetryRef.current?.recordScrollDepth(depth)}
              onRespondSuggestion={respondToPassage}
              onOpenSuggestionTarget={(suggestion) => {
                if (suggestion.tgt_document_id) openDocument(suggestion.tgt_document_id, suggestion.tgt_page);
              }}
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
              {reviewError && <p role="alert" className="reader-action-error">{reviewError} Refresh the assertion preview before trying again.</p>}
              {!loading && mentalLinks.length === 0 && <PanelEmpty message="No argument-level links yet. They appear after at least two documents have mental models." />}
              {mentalLinks.map((link) => {
                const pair = groundedLinkPresentation(link);
                if (!pair) return null;
                const otherTitle = link.source_document_id === docId ? link.target_document_title : link.source_document_title;
                return (
                  <ConnectionCard
                    key={link.id}
                    relation={link.link_type}
                    score={link.confidence}
                    source={otherTitle}
                    explanation={pair.prompt}
                    evidence={pair.evidence}
                    status={link.status}
                    allowConfirm={false}
                    showActions={false}
                    onConfirm={() => previewMentalLink(link.id)}
                    onReject={() => previewMentalLink(link.id)}
                  />
                );
              })}
              {mentalLinks.map(link => <div key={`review-${link.id}`}>
                <button type="button" onClick={() => previewMentalLink(link.id)} aria-label={`Preview grounded assertion from ${link.source_document_title} to ${link.target_document_title}`}>Review assertion · {link.source_document_title} → {link.target_document_title}</button>
                {reviewPreview?.id === link.id && (docId === link.source_document_id || docId === link.target_document_id) && <ReviewAssertion preview={reviewPreview} documentId={docId} label={reviewLabel} reason={reviewReason} busy={reviewBusy} onLabel={setReviewLabel} onReason={setReviewReason} onAct={respondToMentalLink} />}
              </div>)}
            </>
          ) : (
            <>
              <div className="match-group-lbl">Passage-level matches · {loading ? "…" : suggestions.length}</div>
              {suggestionActionError && <div className="reader-action-error">Could not save response: {suggestionActionError}</div>}
              {!loading && suggestionError && <PanelEmpty message={`Suggested passages could not be loaded: ${suggestionError}`} />}
              {!loading && !suggestionError && suggestions.length === 0 && <PanelEmpty message="No passage matches were generated for this document." />}
              {suggestions.map((suggestion) => (
                <ConnectionCard
                  key={suggestion.id}
                  relation="unclassified"
                  score={suggestion.similarity}
                  source={`${suggestion.src_doc} · p.${suggestion.src_page} → ${suggestion.tgt_doc} · p.${suggestion.tgt_page}`}
                  explanation="Similarity-only passage match; not a verified relationship. Compare the two sources before drawing a conclusion."
                  evidence={`${suggestion.src_doc} · p.${suggestion.src_page}: “${suggestion.src_text}”\n${suggestion.tgt_doc} · p.${suggestion.tgt_page}: “${suggestion.tgt_text}”`}
                  allowConfirm={false}
                  status={suggestion.status}
                  onConfirm={() => respondToPassage(suggestion.id, "confirmed")}
                  onReject={() => respondToPassage(suggestion.id, "rejected")}
                  onReveal={() => {
                    setSuggestionsOn(true);
                    setPageNumber(suggestion.src_page);
                  }}
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

function ConnectionCard({ relation, score, source, explanation, evidence, status, onConfirm, onReject, onReveal, allowConfirm = true, showActions = true }: {
  relation: string;
  score: number;
  source: string;
  explanation: string;
  evidence?: string;
  status: string;
  onConfirm: () => void;
  onReject: () => void;
  onReveal?: () => void;
  allowConfirm?: boolean;
  showActions?: boolean;
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
      {evidence && <details className="connection-evidence"><summary>View both source passages</summary><p style={{ whiteSpace: "pre-line" }}>{evidence}</p></details>}
      {showActions && <div className="foot">
        {onReveal && <button className="reveal" onClick={onReveal}><Icon name="eye" size={11} /> Show highlight</button>}
        {reviewed ? (
          <span className={`review-state ${status}`}><Icon name={status === "rejected" ? "x" : "check"} size={11} /> {status}</span>
        ) : (
          <>
            {allowConfirm && <button className="confirm" onClick={onConfirm}><Icon name="check" size={11} /> Confirm</button>}
            <button onClick={onReject}><Icon name="x" size={11} /> {allowConfirm ? "Reject" : "Dismiss"}</button>
          </>
        )}
      </div>}
    </article>
  );
}

function PanelEmpty({ message }: { message: string }) {
  return <div className="panel-empty"><Icon name="link" size={18} /><span>{message}</span></div>;
}
