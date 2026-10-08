"use client";

import dynamic from "next/dynamic";
import { useSearchParams } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { Icon } from "@/components/ui/Icon";
import { ArticleReader } from "@/components/ArticleReader";
import { createReaderTelemetry, type ReaderTelemetry } from "@/lib/reader-telemetry";
import { createDwellTracker, track, type DwellTracker } from "@/lib/analytics";
import { ConnectionsPanel } from "@/components/ConnectionsPanel";
import { PanelResizer } from "@/components/PanelResizer";
import { isTypingTarget, useReaderLayout } from "@/lib/reader-layout";
import { nextMatchLabel } from "@/lib/format";
import { useSelar } from "@/lib/context";
import { initialZoom, readerSettings, suggestionsOnOpen } from "@/lib/settings";
import {
  clientFetch,
  type Annotation,
  type DocumentContent,
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

export default function ReaderPage() {
  const searchParams = useSearchParams();
  const [docId, setDocId] = useState(searchParams.get("docId") || "");
  const [suggestions, setSuggestions] = useState<LinkSuggestion[]>([]);
  const [mentalLinks, setMentalLinks] = useState<MentalModelLink[]>([]);
  const [mentalModel, setMentalModel] = useState<DocumentMentalModel | null>(null);
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [documentContent, setDocumentContent] = useState<DocumentContent | null>(null);
  const [loading, setLoading] = useState(Boolean(searchParams.get("docId")));
  const [suggestionError, setSuggestionError] = useState("");
  const [suggestionActionError, setSuggestionActionError] = useState("");
  // Defaults from Settings (account preferences; see lib/settings.ts).
  const { preferences } = useSelar();
  const readerDefaults = readerSettings(preferences);
  const [zoom, setZoom] = useState(() => initialZoom(readerDefaults.defaultZoom));
  const [annotationsOn, setAnnotationsOn] = useState(true);
  const [suggestionsOn, setSuggestionsOn] = useState(() => suggestionsOnOpen(preferences));
  const [numPages, setNumPages] = useState(0);
  const [pageNumber, setPageNumber] = useState(() => {
    const requestedPage = Number(searchParams.get("page") || "1");
    return Number.isFinite(requestedPage) ? Math.max(1, requestedPage) : 1;
  });
  const telemetryRef = useRef<ReaderTelemetry | null>(null);
  const panels = useReaderLayout();
  const { layout, toggle: togglePanel } = panels;

  // [ toggles the Library, ] toggles Connections (ignored while typing).
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey || isTypingTarget(event.target)) return;
      if (event.key === "[") { event.preventDefault(); togglePanel("library"); }
      else if (event.key === "]") { event.preventDefault(); togglePanel("connections"); }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [togglePanel]);

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

  // Visible dwell time per page (one event per page left; no-op without consent).
  const dwellRef = useRef<DwellTracker | null>(null);
  useEffect(() => {
    if (!docId || documentContent?.document.status !== "ready") return;
    const dwell = createDwellTracker((page, dwellMs) => track("reader_page_viewed", { document_id: docId, page, dwell_ms: dwellMs }));
    dwellRef.current = dwell;
    const onVisibility = () => (document.visibilityState === "hidden" ? dwell.pause() : dwell.resume());
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      dwell.stop();
      if (dwellRef.current === dwell) dwellRef.current = null;
    };
  }, [docId, documentContent?.document.status]);
  useEffect(() => {
    dwellRef.current?.enter(pageNumber);
  }, [pageNumber, docId, documentContent?.document.status]);

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

  async function respondToPassage(id: string, action: "rejected"): Promise<boolean> {
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

  const reloadLinks = useCallback(async () => {
    const links = await clientFetch<MentalModelLink[]>(`/api/mental-model-links?document_id=${docId}`);
    setMentalLinks(links);
    return links;
  }, [docId]);

  return (
    <div className={`reader${layout.libraryOpen ? " has-library" : ""}${layout.connectionsOpen ? " has-connections" : ""}`}>
      <div
        id="reader-library"
        className="reader-side reader-side-library"
        style={{ width: layout.libraryWidth }}
        hidden={!layout.libraryOpen}
      >
        <Sidebar currentId={docId} onPick={selectDocument} />
      </div>
      {layout.libraryOpen && (
        <PanelResizer
          side="library"
          width={layout.libraryWidth}
          label="Resize library"
          controls="reader-library"
          onResize={(width) => panels.resize("library", width)}
          onReset={() => panels.reset("library")}
        />
      )}

      <main className="doc-pane">
        <div className="doc-toolbar">
          <button
            type="button"
            className={`panel-toggle${layout.libraryOpen ? " on" : ""}`}
            aria-controls="reader-library"
            aria-expanded={layout.libraryOpen}
            aria-keyshortcuts="["
            onClick={() => togglePanel("library")}
            title={`${layout.libraryOpen ? "Hide" : "Show"} library  [`}
          >
            <Icon name="menu" size={13} /><span className="panel-toggle-label">Library</span>
          </button>
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
              title={visibleSuggestions.length === 0 ? "No suggested passages for this document" : currentPageSuggestionCount > 0 ? `${currentPageSuggestionCount} suggestion(s) on this page` : "Go to the next page with a suggestion"}
            >{nextMatchLabel(visibleSuggestions.length, currentPageSuggestionCount)}</button>
          </div>
          <div className="tool-spacer" />
          {mentalModel && <span className="mental-domain-chip">{mentalModel.domain || "Mental model ready"}</span>}
          <button
            type="button"
            className={`panel-toggle${layout.connectionsOpen ? " on" : ""}`}
            aria-controls="reader-connections"
            aria-expanded={layout.connectionsOpen}
            aria-keyshortcuts="]"
            onClick={() => togglePanel("connections")}
            title={`${layout.connectionsOpen ? "Hide" : "Show"} connections  ]`}
          >
            <Icon name="link" size={13} /><span className="panel-toggle-label">Connections</span>
          </button>
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
              highlightColor={readerDefaults.highlightColor}
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

      {layout.connectionsOpen && (
        <PanelResizer
          side="connections"
          width={layout.connectionsWidth}
          label="Resize connections"
          controls="reader-connections"
          onResize={(width) => panels.resize("connections", width)}
          onReset={() => panels.reset("connections")}
        />
      )}
      {/* Kept mounted while hidden so a half-written explanation is not lost. */}
      <div
        id="reader-connections"
        className="reader-side reader-side-connections"
        style={{ width: layout.connectionsWidth }}
        hidden={!layout.connectionsOpen}
      >
      <ConnectionsPanel
        key={docId}
        docId={docId}
        links={mentalLinks}
        loading={loading}
        mentalModel={mentalModel}
        reloadLinks={reloadLinks}
        focusLinkId={searchParams.get("linkId")}
        passageSection={
          <details className="cx-passages">
            <summary>Similar passages ({loading ? "…" : visibleSuggestions.length})</summary>
            <p className="cx-note">Similarity-only passage matches: not verified relationships. Use them to explore, then compare the two sources yourself.</p>
            {suggestionActionError && <div className="reader-action-error">Could not save response: {suggestionActionError}</div>}
            {!loading && suggestionError && <PanelEmpty message={`Suggested passages could not be loaded: ${suggestionError}`} />}
            {!loading && !suggestionError && suggestions.length === 0 && <PanelEmpty message="No similar passages were found for this reading." />}
            {suggestions.map((suggestion) => (
              <ConnectionCard
                key={suggestion.id}
                source={`${suggestion.src_doc} · p.${suggestion.src_page} → ${suggestion.tgt_doc} · p.${suggestion.tgt_page}`}
                evidence={`${suggestion.src_doc} · p.${suggestion.src_page}: “${suggestion.src_text}”\n${suggestion.tgt_doc} · p.${suggestion.tgt_page}: “${suggestion.tgt_text}”`}
                status={suggestion.status}
                onReject={() => respondToPassage(suggestion.id, "rejected")}
                onReveal={() => {
                  setSuggestionsOn(true);
                  setPageNumber(suggestion.src_page);
                }}
              />
            ))}
          </details>
        }
      />
      </div>
      {(layout.libraryOpen || layout.connectionsOpen) && (
        <button
          type="button"
          className="reader-scrim"
          aria-label="Close side panels"
          tabIndex={-1}
          onClick={() => { panels.setOpen("library", false); panels.setOpen("connections", false); }}
        />
      )}
    </div>
  );
}

function ConnectionCard({ source, evidence, status, onReject, onReveal }: {
  source: string;
  evidence: string;
  status: string;
  onReject: () => void;
  onReveal?: () => void;
}) {
  const reviewed = status === "confirmed" || status === "rejected";
  return (
    <article className={`match-card passage-card ${status}`}>
      <div className="connection-source">{source}</div>
      <details className="connection-evidence"><summary>View both passages</summary><p style={{ whiteSpace: "pre-line" }}>{evidence}</p></details>
      <div className="foot">
        {onReveal && <button className="reveal" onClick={onReveal}><Icon name="eye" size={11} /> Show on page</button>}
        {reviewed ? (
          <span className={`review-state ${status}`}><Icon name={status === "rejected" ? "x" : "check"} size={11} /> {status === "rejected" ? "dismissed" : status}</span>
        ) : (
          <button onClick={onReject}><Icon name="x" size={11} /> Dismiss</button>
        )}
      </div>
    </article>
  );
}

function PanelEmpty({ message }: { message: string }) {
  return <div className="panel-empty"><Icon name="link" size={18} /><span>{message}</span></div>;
}
