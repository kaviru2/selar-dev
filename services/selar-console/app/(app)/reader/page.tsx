"use client";

import { ReadingPractice } from "@/components/ReadingPractice";
import dynamic from "next/dynamic";
import { useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { Icon } from "@/components/ui/Icon";
import { ArticleReader } from "@/components/ArticleReader";
import { createReaderTelemetry, type ReaderTelemetry } from "@/lib/reader-telemetry";
import { ConnectionsPanel } from "@/components/ConnectionsPanel";
import { PanelResizer } from "@/components/PanelResizer";
import { SuggestionVisibilityToggle } from "@/components/SuggestionVisibilityToggle";
import { isTypingTarget, useReaderLayout } from "@/lib/reader-layout";
import { nextMatchLabel } from "@/lib/format";
import { useSelar } from "@/lib/context";
import { loadSettings, suggestionVisibility, suggestionsOnOpen } from "@/lib/settings";
import { parsePageTarget, type ScrollAnchor, type ZoomMode } from "@/lib/reader/navigation";
import { browserPositionStore, readerPreferences } from "@/lib/reader/position";
import { createDwellTracker, type DwellTracker } from "@/lib/reader/dwell";
import { createDwellTracker as createAnalyticsDwell, track } from "@/lib/analytics";
import type { NavRequest, ViewerState } from "@/components/reader/PdfViewer";
import {
  clientFetch,
  type Annotation,
  type DocumentContent,
  type DocumentMentalModel,
  type LinkSuggestion,
  type MentalModelLink,
} from "@/lib/api";

const PdfViewer = dynamic(() => import("@/components/reader/PdfViewer"), {
  ssr: false,
  loading: () => <div className="reader-empty">Initializing PDF engine…</div>,
});

/** A page only counts as "viewed" for the study after this much dwell. */
const VIEWED_DWELL_MS = 1500;

interface OpenTarget {
  page: number;
  fraction: number;
  /** True when the page came from the URL or a jump, not from memory. */
  explicit: boolean;
}

export default function ReaderPage() {
  const searchParams = useSearchParams();
  // Reader defaults from Settings (lib/settings.ts, same shape as readerPreferences()).
  const { preferences, user } = useSelar();
  const prefs = useMemo(() => readerPreferences(preferences), [preferences]);
  const positions = useMemo(() => browserPositionStore(), []);

  const [readingVisible, setReadingVisible] = useState(false);
  const [docId, setDocId] = useState(searchParams.get("docId") || "");
  const [suggestions, setSuggestions] = useState<LinkSuggestion[]>([]);
  const [mentalLinks, setMentalLinks] = useState<MentalModelLink[]>([]);
  const [mentalModel, setMentalModel] = useState<DocumentMentalModel | null>(null);
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [documentContent, setDocumentContent] = useState<DocumentContent | null>(null);
  const [loading, setLoading] = useState(Boolean(searchParams.get("docId")));
  const [suggestionError, setSuggestionError] = useState("");
  const [suggestionActionError, setSuggestionActionError] = useState("");
  const [annotationsOn, setAnnotationsOn] = useState(true);
  // Study-relevant: suggestions.show_on_open may be locked per cohort (#103).
  const fallbackSuggestionsOn = suggestionsOnOpen(preferences);
  const [suggestionsOn, setSuggestionsOn] = useState(fallbackSuggestionsOn);
  const [suggestionsLocked, setSuggestionsLocked] = useState(false);
  const [showThumbnails, setShowThumbnails] = useState(prefs.showThumbnails);
  const [viewer, setViewer] = useState<ViewerState>({ page: 1, numPages: 0 });
  const [navRequest, setNavRequest] = useState<NavRequest | null>(null);
  const navSeq = useRef(0);
  const annotationDocRef = useRef(docId);
  useEffect(() => { annotationDocRef.current = docId; }, [docId]);
  const telemetryRef = useRef<ReaderTelemetry | null>(null);
  const panels = useReaderLayout();
  const { layout, toggle: togglePanel, setOpen: setPanelOpen } = panels;

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

  // Where to open the current document: URL (?page / #page) > remembered position > page 1.
  const [openTarget, setOpenTarget] = useState<OpenTarget>(() => {
    const fromUrl = typeof window === "undefined" ? null : parsePageTarget(window.location.search, window.location.hash);
    const id = searchParams.get("docId") || "";
    const remembered = !fromUrl && id && prefs.rememberPosition ? positions.load(id) : null;
    return { page: fromUrl ?? remembered?.page ?? 1, fraction: remembered?.fraction ?? 0, explicit: Boolean(fromUrl) };
  });
  const [zoom, setZoom] = useState<ZoomMode>(() => {
    const id = searchParams.get("docId") || "";
    return (id && prefs.rememberPosition && positions.load(id)?.zoom) || prefs.defaultZoom;
  });
  const zoomRef = useRef(zoom);
  useEffect(() => {
    zoomRef.current = zoom;
  }, [zoom]);

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
    let cancelled = false;
    loadSettings()
      .then((settings) => {
        if (cancelled) return;
        const visibility = suggestionVisibility(settings, fallbackSuggestionsOn);
        setSuggestionsOn(visibility.enabled);
        setSuggestionsLocked(visibility.locked);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [fallbackSuggestionsOn]);

  useEffect(() => {
    if (!docId || !readingVisible || documentContent?.document.status !== "ready") return;

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
  }, [docId, readingVisible, documentContent?.document.status]);

  // Pages count as viewed after a short dwell, so scrolling past a page does not.
  const dwellRef = useRef<DwellTracker | null>(null);
  useEffect(() => {
    const dwell = createDwellTracker({
      thresholdMs: VIEWED_DWELL_MS,
      onViewed: (page) => telemetryRef.current?.visitPage(page),
    });
    // Consent-gated usage analytics (docs/ANALYTICS.md): one event per page left, visible time only.
    const pageDwell = createAnalyticsDwell((page, dwellMs) => {
      track("reader_page_viewed", { document_id: docId || undefined, page, dwell_ms: dwellMs });
    });
    dwellRef.current = {
      ...dwell,
      setPage: (page: number) => { dwell.setPage(page); pageDwell.enter(page); },
      pause: () => { dwell.pause(); pageDwell.pause(); },
      resume: () => { dwell.resume(); pageDwell.resume(); },
    };
    const timer = window.setInterval(() => dwell.tick(), 500);
    const onVisibility = () => (document.hidden ? dwellRef.current?.pause() : dwellRef.current?.resume());
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisibility);
      pageDwell.stop();
      dwellRef.current = null;
    };
  }, [docId]);
  useEffect(() => {
    if (documentContent?.document.status === "ready" && viewer.numPages) dwellRef.current?.setPage(viewer.page);
  }, [viewer.page, viewer.numPages, documentContent?.document.status]);


  useEffect(() => {
    for (const suggestion of suggestions) {
      if (suggestion.status === "pending") telemetryRef.current?.showSuggestion(suggestion.id);
    }
  }, [suggestions]);

  // Keep ?page= in the address bar in step with the reader (no history spam).
  useEffect(() => {
    if (!docId || !viewer.numPages) return;
    const timer = window.setTimeout(() => {
      const params = new URLSearchParams(window.location.search);
      if (params.get("docId") === docId && params.get("page") === String(viewer.page)) return;
      params.set("docId", docId);
      params.set("page", String(viewer.page));
      params.delete("block");
      window.history.replaceState(window.history.state, "", `/reader?${params}`);
    }, 400);
    return () => window.clearTimeout(timer);
  }, [docId, viewer.page, viewer.numPages]);

  const saveTimerRef = useRef(0);
  const savePosition = useCallback((id: string, anchor: ScrollAnchor) => {
    window.clearTimeout(saveTimerRef.current);
    saveTimerRef.current = window.setTimeout(() => positions.save(id, { page: anchor.page, fraction: Math.round(anchor.fraction * 1000) / 1000, zoom: zoomRef.current }), 300);
  }, [positions]);
  const onPositionChange = useCallback((anchor: ScrollAnchor) => {
    if (docId && prefs.rememberPosition) savePosition(docId, anchor);
  }, [docId, prefs.rememberPosition, savePosition]);

  const onScrollDepth = useCallback((depth: number) => telemetryRef.current?.recordScrollDepth(depth), []);

  const openDocument = useCallback((id: string, page?: number, flash?: Omit<NavRequest, "id" | "page">) => {
    const remembered = page === undefined && prefs.rememberPosition ? positions.load(id) : null;
    const target = page ?? remembered?.page ?? 1;
    setLoading(true);
    setSuggestionError("");
    setSuggestionActionError("");
    setOpenTarget({ page: Math.max(1, target), fraction: remembered?.fraction ?? 0, explicit: page !== undefined });
    setZoom((prefs.rememberPosition && positions.load(id)?.zoom) || prefs.defaultZoom);
    setViewer({ page: Math.max(1, target), numPages: 0 });
    setNavRequest(flash && page ? { id: ++navSeq.current, page, ...flash } : null);
    setSuggestions([]);
    setMentalLinks([]);
    setMentalModel(null);
    setAnnotations([]);
    setDocumentContent(null);
    setDocId(id);
    const params = new URLSearchParams({ docId: id });
    if (page) params.set("page", String(page));
    if (id !== docId) window.history.pushState(null, "", `/reader?${params}`);
  }, [docId, positions, prefs.defaultZoom, prefs.rememberPosition]);

  const selectDocument = useCallback((id: string) => {
    if (id !== docId) openDocument(id);
  }, [docId, openDocument]);

  const jumpTo = useCallback((page: number, flash?: Omit<NavRequest, "id" | "page">) => {
    setNavRequest({ id: ++navSeq.current, page, ...flash });
    // On narrow windows the panels float over the document; get them out of the way.
    if (window.matchMedia?.("(max-width: 800px)").matches) {
      setPanelOpen("library", false);
      setPanelOpen("connections", false);
    }
  }, [setPanelOpen]);

  const visibleSuggestions = suggestions.filter((item) => item.status !== "rejected");
  const currentPageSuggestionCount = visibleSuggestions.filter((item) => item.src_page === viewer.page).length;
  const isPdf = !documentContent || documentContent.document.source_type === "pdf";
  const ingestionState = documentContent?.document.status;
  const reviewCount = mentalLinks.filter((link) => link.status === "candidate").length;

  const goToNextSuggestion = useCallback(() => {
    const ordered = suggestions
      .filter((item) => item.status !== "rejected" && item.src_page > 0)
      .sort((left, right) => left.src_page - right.src_page);
    if (!ordered.length) return;
    const next = ordered.find((item) => item.src_page > viewer.page) || ordered[0];
    if (!suggestionsLocked) setSuggestionsOn(true);
    jumpTo(next.src_page, { quote: next.src_text, bboxes: next.src_bboxes });
  }, [viewer.page, suggestions, jumpTo, suggestionsLocked]);

  const createAnnotation = useCallback(async (
    type: string,
    bboxes: Array<{ x: number; y: number; w: number; h: number }>,
    color: string,
    page: number,
    comment = "",
    anchor?: Annotation["anchor"],
  ) => {
    const annotation = await clientFetch<Annotation>("/api/annotations", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ document_id: docId, page, bbox: bboxes, type, color, comment, anchor }),
    });
    if (annotationDocRef.current === docId) setAnnotations((current) => [...current, annotation]);
  }, [docId]);

  const updateAnnotation = useCallback(async (annotation: Annotation, color: Annotation["color"], comment: string) => {
    const saved = await clientFetch<Annotation>(`/api/annotations/${annotation.id}`, {
      method: "PATCH", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ color, comment, source_hash: annotation.anchor?.source_hash }),
    });
    if (annotationDocRef.current === annotation.document_id) setAnnotations(current => current.map(a => a.id === saved.id ? saved : a));
  }, []);

  const deleteAnnotation = useCallback(async (annotation: Annotation) => {
    await clientFetch(`/api/annotations/${annotation.id}`, { method: "DELETE" });
    if (annotationDocRef.current === annotation.document_id) setAnnotations(current => current.filter(a => a.id !== annotation.id));
  }, []);

  // Passage matches are similarity-only (relation "unclassified"); the API
  // refuses to confirm them, so the reader can only dismiss them (#96).
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

  // Compare-step "open ↗" for a passage in this same document: scroll in place
  // and flash it, instead of reloading the app and losing the guided card.
  const openWitness = useCallback((targetDocId: string, locator: { page?: number; block_index?: number } | undefined, quote: string) => {
    if (targetDocId !== docId || !isPdf || !locator?.page) return false;
    jumpTo(locator.page, { quote });
    return true;
  }, [docId, isPdf, jumpTo]);

  const onViewerState = useCallback((state: ViewerState) => setViewer(state), []);

  const markToggles = (
    <div className="rd-grp">
      <button type="button" className={annotationsOn ? "on" : ""} aria-pressed={annotationsOn} aria-label="Show my marks" title="Show my marks" onClick={() => setAnnotationsOn((value) => !value)}>
        <Icon name="highlight" size={12} /> <span className="rd-hide-sm">Marks</span>
      </button>
      <SuggestionVisibilityToggle
        enabled={suggestionsOn}
        locked={suggestionsLocked}
        count={visibleSuggestions.length}
        onToggle={() => setSuggestionsOn((value) => !value)}
      />
      <button
        type="button"
        aria-label="Go to next suggested passage"
        disabled={visibleSuggestions.length === 0}
        onClick={goToNextSuggestion}
        title={visibleSuggestions.length === 0 ? "No suggested passages for this document" : currentPageSuggestionCount > 0 ? `${currentPageSuggestionCount} suggestion(s) on this page` : "Go to the next page with a suggestion"}
      >
        {nextMatchLabel(visibleSuggestions.length, currentPageSuggestionCount)}
      </button>
    </div>
  );

  const libraryToggle = (
    <button
      type="button"
      className={`panel-toggle${layout.libraryOpen ? " on" : ""}`}
      aria-controls="reader-library"
      aria-expanded={layout.libraryOpen}
      aria-keyshortcuts="["
      onClick={() => togglePanel("library")}
      title={`${layout.libraryOpen ? "Hide" : "Show"} library  [`}
    >
      <Icon name="book" size={13} /><span className="panel-toggle-label">Library</span>
    </button>
  );
  const toolbarEnd = (
    <>
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
        <Icon name="link" size={13} /><span className="panel-toggle-label">Connections{reviewCount ? ` (${reviewCount})` : ""}</span>
      </button>
    </>
  );
  const showViewer = Boolean(docId && isPdf && !loading && ingestionState !== "processing" && ingestionState !== "failed");

  return (
    <ReadingPractice key={docId} documentId={ingestionState === "ready" ? docId : ""} onReadingChange={setReadingVisible}>
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
        {!showViewer && (
          <div className="doc-toolbar">
            {libraryToggle}
            <div className="tool-spacer" />
            {toolbarEnd}
          </div>
        )}
        {loading ? (
          <div className="pdf-container"><div className="reader-empty">Loading source snapshot…</div></div>
        ) : ingestionState === "processing" ? (
          <div className="pdf-container"><div className="reader-empty"><Icon name="spinner" size={20} className="animate-spin" /><strong>Processing this source</strong><span>You can return to the Library while the durable ingestion job runs.</span></div></div>
        ) : ingestionState === "failed" ? (
          <div className="pdf-container"><div className="reader-empty"><Icon name="x" size={20} /><strong>Source processing failed</strong><span>{documentContent?.document.ingestion_error || "Open the Library to retry this document."}</span></div></div>
        ) : docId && !isPdf && documentContent ? (
          <div className="article-container">
            <ArticleReader content={documentContent} targetBlock={searchParams.has("block") ? Number(searchParams.get("block")) : undefined} />
          </div>
        ) : docId ? (
          <PdfViewer
            key={docId}
            docId={docId}
            initialPage={openTarget.page}
            initialFraction={openTarget.explicit ? 0 : openTarget.fraction}
            zoom={zoom}
            onZoomChange={setZoom}
            navRequest={navRequest}
            annotationsOn={annotationsOn}
            suggestionsOn={suggestionsOn}
            suggestions={suggestions}
            annotations={annotations}
            showThumbnails={showThumbnails}
            highlightColor={prefs.highlightColor}
            onToggleThumbnails={() => setShowThumbnails((value) => !value)}
            toolbarStart={libraryToggle}
            toolbarExtras={markToggles}
            toolbarEnd={toolbarEnd}
            onStateChange={onViewerState}
            onPositionChange={onPositionChange}
            onScrollDepth={onScrollDepth}
            onCreateAnnotation={createAnnotation}
            onUpdateAnnotation={updateAnnotation}
            onDeleteAnnotation={deleteAnnotation}
            sourceHash={documentContent?.document.content_hash || ""}
            userId={user?.id}
            onRespondSuggestion={respondToPassage}
            onOpenSuggestionTarget={(suggestion) => {
              if (!suggestion.tgt_document_id) return;
              if (suggestion.tgt_document_id === docId) jumpTo(suggestion.tgt_page, { quote: suggestion.tgt_text });
              else openDocument(suggestion.tgt_document_id, suggestion.tgt_page, { quote: suggestion.tgt_text });
            }}
          />
        ) : (
          <div className="pdf-container">
            <div className="reader-empty">
              <Icon name="book" size={24} />
              <strong>Select a document to begin reading</strong>
              <span>SELAR will surface evidence-backed connections here.</span>
            </div>
          </div>
        )}
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
          onOpenWitness={openWitness}
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
                  onReveal={isPdf ? () => {
                    if (!suggestionsLocked) setSuggestionsOn(true);
                    jumpTo(suggestion.src_page, { quote: suggestion.src_text, bboxes: suggestion.src_bboxes });
                  } : undefined}
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
          onClick={() => { setPanelOpen("library", false); setPanelOpen("connections", false); }}
        />
      )}
    </div>
    </ReadingPractice>
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
      <details className="connection-evidence"><summary>View both passages</summary><p>{evidence}</p></details>
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
