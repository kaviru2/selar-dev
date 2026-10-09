"use client";

// PdfViewer — continuous-scroll PDF reader with virtualised page rendering.
//
// Navigation model (one, and only one): all pages sit in a single vertical
// column. Scrolling, the page box, prev/next, keyboard shortcuts, thumbnails,
// deep links (?page=N / #page=N) and "jump to passage" all resolve to a
// scrollTop on that column (see lib/reader/navigation.ts). Only pages near the
// viewport mount a canvas + text layer; the rest are sized placeholders, so
// moving between pages never re-renders the whole document.

import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import type { PDFDocumentProxy } from "pdfjs-dist";
import { Icon } from "@/components/ui/Icon";
import { visibleSuggestionBoxes } from "@/lib/reader-highlights";
import type { Annotation, LinkSuggestion } from "@/lib/api";
import type { TextAnchor } from "@/lib/reader/annotation-anchor";
import { annotationBoxes, captureSelectionAnchor } from "@/lib/reader/annotation-dom";
import { AnnotationEditor, HighlightsList, annotationColors } from "./AnnotationTools";
import {
  buildLayout,
  captureAnchor,
  clampPage,
  keyAction,
  pageAtOffset,
  renderRange,
  resolveScale,
  scrollTopForAnchor,
  scrollTopForPage,
  scrollTopForPoint,
  stepZoom,
  type Layout,
  type PageSize,
  type ScrollAnchor,
  type ZoomMode,
} from "@/lib/reader/navigation";
import { locateQuote, markMatches } from "@/lib/reader/text-anchor";
import { mergeLineRects, normalizeRects, parseBBoxes, type NormalizedBBox } from "@/lib/reader/rects";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";
import "./reader.css";
import "./annotation-tools.css";

pdfjs.GlobalWorkerOptions.workerSrc = new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url).toString();

const GAP = 16;
const PADDING = 24;
const OVERSCAN = 2;
const DEFAULT_SIZE: PageSize = { width: 612, height: 792 };

export interface NavRequest {
  /** Increment to issue a new request. */
  id: number;
  page: number;
  /** Text to find and flash on the target page. */
  quote?: string;
  /** User marks use a full source-bound anchor rather than a best-effort quote. */
  anchor?: TextAnchor;
  /** Fallback boxes (0..1) to flash when the quote cannot be located. */
  bboxes?: NormalizedBBox[] | string;
}

export interface ViewerState {
  page: number;
  numPages: number;
}

export interface PdfViewerProps {
  docId: string;
  initialPage: number;
  initialFraction?: number;
  zoom: ZoomMode;
  onZoomChange: (zoom: ZoomMode) => void;
  navRequest: NavRequest | null;
  annotationsOn: boolean;
  suggestionsOn: boolean;
  suggestions: LinkSuggestion[];
  annotations: Annotation[];
  showThumbnails: boolean;
  onToggleThumbnails: () => void;
  /** Colour for new highlights (Settings › Reader › Highlight colour). */
  highlightColor?: string;
  toolbarStart?: ReactNode;
  toolbarExtras?: ReactNode;
  toolbarEnd?: ReactNode;
  onStateChange: (state: ViewerState) => void;
  onPositionChange?: (anchor: ScrollAnchor) => void;
  onScrollDepth?: (depth: number) => void;
  sourceHash?: string;
  onCreateAnnotation?: (type: string, bboxes: NormalizedBBox[], color: string, page: number, comment?: string, anchor?: TextAnchor) => Promise<void>;
  onUpdateAnnotation?: (annotation: Annotation, color: Annotation["color"], comment: string) => Promise<void>;
  onDeleteAnnotation?: (annotation: Annotation) => Promise<void>;
  /** Passage matches are similarity-only; the reader can only dismiss them (#96). */
  onRespondSuggestion: (id: string, action: "rejected") => Promise<boolean>;
  onOpenSuggestionTarget: (suggestion: LinkSuggestion) => void;
}

type Pending =
  | { kind: "page"; page: number; fraction?: number }
  | { kind: "anchor"; anchor: ScrollAnchor; offset: number };

interface Flash {
  key: number;
  page: number;
  boxes: NormalizedBBox[];
}

interface SelectionMenuState {
  anchor: TextAnchor | null;
  quote: string;
  page: number;
  boxes: NormalizedBBox[];
  x: number;
  y: number;
}

interface PopoverState {
  suggestionId: string;
  page: number;
  /** Position inside the page element, in px. */
  x: number;
  y: number;
  above: boolean;
}

function isEditable(target: EventTarget | null): boolean {
  const element = target as HTMLElement | null;
  if (!element || !element.tagName) return false;
  return element.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName);
}

function prefersReducedMotion(): boolean {
  return typeof window !== "undefined" && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
}

function textSegments(pageElement: Element): HTMLElement[] {
  return Array.from(pageElement.querySelectorAll<HTMLElement>(".textLayer span")).filter(
    (span) => !span.classList.contains("markedContent") && !span.querySelector("span"),
  );
}

export default function PdfViewer(props: PdfViewerProps) {
  const {
    docId, initialPage, initialFraction = 0, zoom, onZoomChange, navRequest, annotationsOn, suggestionsOn,
    suggestions, annotations, showThumbnails, onToggleThumbnails, highlightColor = "yellow", toolbarStart, toolbarExtras, toolbarEnd, onStateChange,
    onPositionChange, onScrollDepth, onCreateAnnotation, onRespondSuggestion, onOpenSuggestionTarget,
    sourceHash = "", onUpdateAnnotation, onDeleteAnnotation,
  } = props;

  const rootRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  // The scroll column mounts only after the PDF loads (it lives inside <Document>),
  // so effects that observe it key off this state rather than the ref.
  const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
  const attachScroll = useCallback((element: HTMLDivElement | null) => {
    scrollRef.current = element;
    setScrollElement(element);
  }, []);
  const pdfRef = useRef<PDFDocumentProxy | null>(null);
  const pendingRef = useRef<Pending | null>({ kind: "page", page: initialPage, fraction: initialFraction });
  const expectedScrollRef = useRef<number | null>(null);
  const stickyPageRef = useRef<number | null>(null);
  const textCacheRef = useRef(new Map<number, string>());
  const pendingFlashRef = useRef<NavRequest | null>(null);
  const pendingFindRef = useRef<{ page: number; nth: number } | null>(null);
  const rafRef = useRef(0);

  const [numPages, setNumPages] = useState(0);
  const [sizes, setSizes] = useState<PageSize[]>([]);
  const [sizesReady, setSizesReady] = useState(false);
  const [viewport, setViewport] = useState({ width: 0, height: 0 });
  const [range, setRange] = useState<[number, number]>([1, 0]);
  const [currentPage, setCurrentPage] = useState(initialPage);
  const [pageInput, setPageInput] = useState<string | null>(null);
  const [textReady, setTextReady] = useState<Record<number, number>>({});
  const [flash, setFlash] = useState<Flash | null>(null);
  const [selectionMenu, setSelectionMenu] = useState<SelectionMenuState | null>(null);
  const [selectionColor, setSelectionColor] = useState<Annotation["color"]>(annotationColors.includes(highlightColor as Annotation["color"]) ? highlightColor as Annotation["color"] : "yellow");
  const [annotationBusy, setAnnotationBusy] = useState(false);
  const [annotationError, setAnnotationError] = useState("");
  const [highlightsOpen, setHighlightsOpen] = useState(false);
  const [editor, setEditor] = useState<{ annotation?: Annotation; selection?: SelectionMenuState } | null>(null);
  const [markBoxes, setMarkBoxes] = useState<Record<string, NormalizedBBox[]>>({});
  useLayoutEffect(() => {
    const boxes: Record<string, NormalizedBBox[]> = {};
    for (const a of annotations) boxes[a.id] = annotationBoxes(rootRef.current?.querySelector(`.rd-page[data-page="${a.page}"]`) || null, a, sourceHash);
    // DOM geometry is only available after the PDF text layer commits; measure before paint.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMarkBoxes(boxes);
  }, [annotations, sourceHash, textReady, zoom, range]);
  const [popover, setPopover] = useState<PopoverState | null>(null);
  const [popoverBusy, setPopoverBusy] = useState(false);
  const [popoverError, setPopoverError] = useState("");
  const [findOpen, setFindOpen] = useState(false);
  const [findQuery, setFindQuery] = useState("");
  const [findActive, setFindActive] = useState("");
  const [findMatches, setFindMatches] = useState<Array<{ page: number; nth: number }>>([]);
  const [findIndex, setFindIndex] = useState(-1);
  const [findBusy, setFindBusy] = useState(false);
  const [findTick, setFindTick] = useState(0);
  const [announce, setAnnounce] = useState("");
  const findInputRef = useRef<HTMLInputElement>(null);

  const scale = useMemo(() => resolveScale(zoom, sizes[0], viewport, PADDING), [zoom, sizes, viewport]);
  const layout: Layout = useMemo(() => buildLayout(sizes, scale, { gap: GAP, padding: PADDING }), [sizes, scale]);
  const layoutRef = useRef(layout);
  const numPagesRef = useRef(0);

  // ——— Document load: page sizes for the whole column ———
  const onDocumentLoad = useCallback((pdf: PDFDocumentProxy) => {
    pdfRef.current = pdf;
    textCacheRef.current.clear();
    setNumPages(pdf.numPages);
    numPagesRef.current = pdf.numPages;
    setSizes(Array.from({ length: pdf.numPages }, () => DEFAULT_SIZE));
    setSizesReady(false);
    Promise.all(Array.from({ length: pdf.numPages }, (_, index) => pdf.getPage(index + 1).then((page) => {
      const view = page.getViewport({ scale: 1 });
      return { width: view.width, height: view.height };
    }))).then((all) => {
      if (pdfRef.current !== pdf) return;
      setSizes(all);
      setSizesReady(true);
    }).catch(() => setSizesReady(true));
  }, []);

  // ——— Viewport size (fit-width/fit-page follow it, keeping position) ———
  useEffect(() => {
    const element = scrollElement;
    if (!element) return;
    const observer = new ResizeObserver(() => {
      const next = { width: element.clientWidth, height: element.clientHeight };
      setViewport((previous) => {
        if (previous.width === next.width && previous.height === next.height) return previous;
        if (previous.width > 0 && !pendingRef.current) {
          pendingRef.current = { kind: "anchor", anchor: captureAnchor(layoutRef.current, element.scrollTop), offset: 0 };
        }
        return next;
      });
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [scrollElement]);

  const updateFromScroll = useCallback(() => {
    const element = scrollRef.current;
    const current = layoutRef.current;
    if (!element || !current.tops.length) return;
    const { scrollTop, clientHeight, scrollHeight } = element;
    if (expectedScrollRef.current !== null && Math.abs(scrollTop - expectedScrollRef.current) > 2) {
      stickyPageRef.current = null;
      expectedScrollRef.current = null;
    }
    const [first, last] = renderRange(current, scrollTop, clientHeight, OVERSCAN);
    setRange((previous) => (previous[0] === first && previous[1] === last ? previous : [first, last]));
    const page = stickyPageRef.current ?? pageAtOffset(current, scrollTop, clientHeight);
    setCurrentPage(page);
    const scrollable = scrollHeight - clientHeight;
    onScrollDepth?.(scrollable > 0 ? (scrollTop / scrollable) * 100 : 100);
    onPositionChange?.(captureAnchor(current, scrollTop));
  }, [onScrollDepth, onPositionChange]);

  // Apply pending scroll targets once the layout they refer to exists.
  useLayoutEffect(() => {
    layoutRef.current = layout;
    const element = scrollRef.current;
    const pending = pendingRef.current;
    if (!element || !layout.tops.length || viewport.width === 0) return;
    if (pending && (sizesReady || pending.kind === "anchor")) {
      let target = 0;
      if (pending.kind === "page") {
        const page = clampPage(pending.page, layout.tops.length);
        target = pending.fraction ? scrollTopForAnchor(layout, { page, fraction: pending.fraction }) : scrollTopForPage(layout, page);
        // A restored mid-page position reports whatever page is under the reading line;
        // an explicit page jump keeps that page as current until the user scrolls.
        stickyPageRef.current = pending.fraction ? null : page;
      } else {
        target = Math.max(0, scrollTopForAnchor(layout, pending.anchor) - pending.offset);
        stickyPageRef.current = null;
      }
      element.scrollTop = target;
      expectedScrollRef.current = element.scrollTop;
      pendingRef.current = null;
    }
    updateFromScroll();
  }, [layout, sizesReady, viewport.width, updateFromScroll]);

  const onScroll = useCallback(() => {
    if (rafRef.current) return;
    rafRef.current = requestAnimationFrame(() => {
      rafRef.current = 0;
      updateFromScroll();
    });
  }, [updateFromScroll]);

  useEffect(() => () => cancelAnimationFrame(rafRef.current), []);

  // Report state; keep the page box in sync unless the user is typing in it.
  useEffect(() => {
    onStateChange({ page: currentPage, numPages });
  }, [currentPage, numPages, onStateChange]);

  const goToPage = useCallback((page: number, fraction = 0) => {
    const element = scrollRef.current;
    const current = layoutRef.current;
    const target = clampPage(page, numPagesRef.current);
    if (!element || !current.tops.length) {
      pendingRef.current = { kind: "page", page: target, fraction };
      return;
    }
    element.scrollTop = fraction ? scrollTopForAnchor(current, { page: target, fraction }) : scrollTopForPage(current, target);
    expectedScrollRef.current = element.scrollTop;
    stickyPageRef.current = fraction ? null : target;
    setAnnounce(`Page ${target} of ${numPagesRef.current}`);
    updateFromScroll();
  }, [updateFromScroll]);

  const zoomTo = useCallback((next: ZoomMode, offset = 0) => {
    const element = scrollRef.current;
    if (element) pendingRef.current = { kind: "anchor", anchor: captureAnchor(layoutRef.current, element.scrollTop + offset), offset };
    onZoomChange(next);
  }, [onZoomChange]);

  // ——— Navigation requests from outside (suggestions, compare quotes, deep links) ———
  useEffect(() => {
    if (!navRequest) return;
    pendingFlashRef.current = navRequest.quote || navRequest.bboxes ? navRequest : null;
    goToPage(navRequest.page);
  }, [navRequest, goToPage]);

  // Once the target page's text layer exists, find the passage, flash it and centre it.
  useEffect(() => {
    const request = pendingFlashRef.current;
    if (!request) return;
    const pageElement = rootRef.current?.querySelector<HTMLElement>(`.rd-page[data-page="${request.page}"]`);
    const ready = textReady[request.page];
    if (!pageElement || !ready) return;
    pendingFlashRef.current = null;
    const pageRect = pageElement.getBoundingClientRect();
    if (request.anchor && request.anchor.source_hash !== sourceHash) return;
    let boxes: NormalizedBBox[] = request.anchor ? annotationBoxes(pageElement, { anchor: request.anchor, bbox: request.bboxes || [] } as Annotation, sourceHash) : [];
    if (request.quote && !request.anchor) {
      const spans = textSegments(pageElement);
      const match = locateQuote(spans.map((span) => span.textContent || ""), request.quote);
      if (match) {
        const range = document.createRange();
        const startNode = spans[match.start.segment].firstChild;
        const endNode = spans[match.end.segment].firstChild;
        if (startNode && endNode) {
          range.setStart(startNode, Math.min(match.start.offset, startNode.textContent?.length ?? 0));
          range.setEnd(endNode, Math.min(match.end.offset, endNode.textContent?.length ?? 0));
          boxes = normalizeRects(mergeLineRects(Array.from(range.getClientRects())), pageRect);
        }
      }
    }
    if (!boxes.length && request.bboxes) boxes = parseBBoxes(request.bboxes).filter((box) => box.w * box.h < 0.5);
    if (!boxes.length) return;
    setFlash({ key: request.id, page: request.page, boxes });
    const element = scrollRef.current;
    if (element) {
      element.scrollTop = scrollTopForPoint(layoutRef.current, request.page, boxes[0].y, element.clientHeight);
      expectedScrollRef.current = element.scrollTop;
      stickyPageRef.current = request.page;
      updateFromScroll();
    }
  }, [textReady, range, navRequest, updateFromScroll, sourceHash]);

  useEffect(() => {
    if (!flash) return;
    const timer = window.setTimeout(() => setFlash(null), prefersReducedMotion() ? 4000 : 2600);
    return () => window.clearTimeout(timer);
  }, [flash]);

  // ——— Find in document (Ctrl/Cmd+F) ———
  const pageText = useCallback(async (page: number) => {
    const cached = textCacheRef.current.get(page);
    if (cached !== undefined) return cached;
    const pdf = pdfRef.current;
    if (!pdf) return "";
    const content = await (await pdf.getPage(page)).getTextContent();
    const text = content.items.map((item) => ("str" in item ? item.str : "")).join(" ");
    textCacheRef.current.set(page, text);
    return text;
  }, []);

  useEffect(() => {
    const query = findQuery.trim();
    if (!findOpen || query.length < 2) return;
    let cancelled = false;
    const timer = window.setTimeout(async () => {
      setFindBusy(true);
      const needle = query.toLowerCase();
      const found: Array<{ page: number; nth: number }> = [];
      for (let page = 1; page <= numPagesRef.current && !cancelled; page++) {
        const haystack = (await pageText(page)).toLowerCase();
        let nth = 0;
        for (let at = haystack.indexOf(needle); at >= 0; at = haystack.indexOf(needle, at + needle.length)) found.push({ page, nth: nth++ });
      }
      if (cancelled) return;
      setFindBusy(false);
      setFindActive(query);
      setFindMatches(found);
      const start = found.findIndex((match) => match.page >= currentPage);
      setFindIndex(found.length ? Math.max(0, start) : -1);
      if (found.length) {
        const first = found[Math.max(0, start)];
        pendingFindRef.current = first;
        goToPage(first.page);
      }
      setAnnounce(found.length ? `${found.length} matches` : "No matches");
    }, 250);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
    // currentPage is read only to pick the first match; re-running on scroll would reset the search.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [findQuery, findOpen, pageText, goToPage]);

  const stepFind = useCallback((direction: 1 | -1) => {
    if (!findMatches.length) return;
    const next = (findIndex + direction + findMatches.length) % findMatches.length;
    setFindIndex(next);
    pendingFindRef.current = findMatches[next];
    setFindTick((tick) => tick + 1);
    goToPage(findMatches[next].page);
  }, [findMatches, findIndex, goToPage]);

  useEffect(() => {
    const target = pendingFindRef.current;
    if (!target) return;
    const pageElement = rootRef.current?.querySelector<HTMLElement>(`.rd-page[data-page="${target.page}"]`);
    if (!pageElement || !textReady[target.page]) return;
    const marks = pageElement.querySelectorAll<HTMLElement>("mark.rd-find");
    if (!marks.length) return;
    pendingFindRef.current = null;
    rootRef.current?.querySelectorAll(".rd-find-current").forEach((mark) => mark.classList.remove("rd-find-current"));
    const mark = marks[Math.min(target.nth, marks.length - 1)];
    mark.classList.add("rd-find-current");
    const element = scrollRef.current;
    if (element) {
      const pageRect = pageElement.getBoundingClientRect();
      const y = (mark.getBoundingClientRect().top - pageRect.top) / pageRect.height;
      element.scrollTop = scrollTopForPoint(layoutRef.current, target.page, y, element.clientHeight);
      expectedScrollRef.current = element.scrollTop;
      stickyPageRef.current = target.page;
      updateFromScroll();
    }
  }, [textReady, range, findActive, findTick, updateFromScroll]);

  const onTextReady = useCallback((page: number) => setTextReady((ready) => ({ ...ready, [page]: (ready[page] || 0) + 1 })), []);

  const findRenderer = useCallback(({ str }: { str: string }) => markMatches(str, findActive), [findActive]);

  const closeFind = useCallback(() => {
    setFindOpen(false);
    setFindQuery("");
    setFindMatches([]);
    setFindIndex(-1);
    setFindActive("");
    scrollRef.current?.focus({ preventScroll: true });
  }, []);

  // ——— Keyboard ———
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      const root = rootRef.current;
      const target = event.target;
      // Keys typed elsewhere (Connections panel, library filter) are not for the viewer.
      const inViewer = !!root && (!(target instanceof Node) || root.contains(target) || target === document.body || target === document.documentElement);
      if (!inViewer) return;
      if (event.key === "Escape") {
        if (popover) { setPopover(null); event.preventDefault(); return; }
        if (selectionMenu) { setSelectionMenu(null); window.getSelection()?.removeAllRanges(); event.preventDefault(); return; }
        if (findOpen) { closeFind(); event.preventDefault(); return; }
      }
      const action = keyAction({
        key: event.key, ctrlKey: event.ctrlKey, metaKey: event.metaKey, altKey: event.altKey, shiftKey: event.shiftKey,
        inEditable: isEditable(event.target),
      });
      if (!action) return;
      event.preventDefault();
      switch (action) {
        case "next": goToPage(currentPage + 1); break;
        case "prev": goToPage(currentPage - 1); break;
        case "first": goToPage(1); break;
        case "last": goToPage(numPagesRef.current); break;
        case "find":
          setFindOpen(true);
          requestAnimationFrame(() => findInputRef.current?.select());
          break;
        case "zoomIn": zoomTo(stepZoom(scale, 1)); break;
        case "zoomOut": zoomTo(stepZoom(scale, -1)); break;
        case "zoomReset": zoomTo("fit-width"); break;
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [currentPage, goToPage, zoomTo, scale, popover, selectionMenu, findOpen, closeFind]);

  // Ctrl/⌘ + wheel (and trackpad pinch) zooms around the pointer instead of the browser page.
  useEffect(() => {
    const element = scrollElement;
    if (!element) return;
    let pendingScale = 0;
    let frame = 0;
    function onWheel(event: WheelEvent) {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      const base = pendingScale || scale;
      pendingScale = Math.min(4, Math.max(0.25, base * Math.exp(-event.deltaY * 0.01)));
      const offset = event.clientY - element!.getBoundingClientRect().top;
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        zoomTo(Math.round(pendingScale * 100) / 100, offset);
        pendingScale = 0;
      });
    }
    element.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      element.removeEventListener("wheel", onWheel);
      cancelAnimationFrame(frame);
    };
  }, [scale, zoomTo, scrollElement]);

  // ——— Selection: capture immutable quote/offsets before focus moves to controls. ———
  useEffect(() => {
    function onSelectionEnd(event: Event) {
      const target = event.target as HTMLElement | null;
      if (target?.closest(".rd-selection-menu, .rd-popover, .rd-annotation-editor, .rd-highlights") || editor || annotationBusy) return;
      if (event instanceof KeyboardEvent && !event.shiftKey) return;
      requestAnimationFrame(() => {
        const selection = window.getSelection();
        if (!selection || selection.isCollapsed || !selection.rangeCount) { setSelectionMenu(null); return; }
        const range = selection.getRangeAt(0);
        const startPage = range.startContainer.parentElement?.closest<HTMLElement>(".rd-page");
        const endPage = range.endContainer.parentElement?.closest<HTMLElement>(".rd-page");
        if (!startPage || startPage !== endPage || !rootRef.current?.contains(startPage)) { setSelectionMenu(null); return; }
        const page = Number(startPage.dataset.page);
        const pageRect = startPage.getBoundingClientRect();
        const boxes = normalizeRects(mergeLineRects(Array.from(range.getClientRects())), pageRect);
        if (!boxes.length) return;
        const last = boxes[boxes.length - 1];
        const anchor = captureSelectionAnchor(startPage, range, sourceHash);
        setPopover(null); setAnnotationError("");
        setSelectionMenu({ page, boxes, anchor, quote: anchor?.exact || range.toString(),
          x: Math.max(0, Math.min(last.x * pageRect.width, pageRect.width - 320)),
          y: (last.y + last.h) * pageRect.height + 6 });
      });
    }
    document.addEventListener("pointerup", onSelectionEnd);
    document.addEventListener("keyup", onSelectionEnd);
    return () => { document.removeEventListener("pointerup", onSelectionEnd); document.removeEventListener("keyup", onSelectionEnd); };
  }, [sourceHash, editor, annotationBusy]);

  async function createFromSelection() {
    if (!selectionMenu?.anchor || !onCreateAnnotation || annotationBusy) return;
    setAnnotationBusy(true); setAnnotationError("");
    try {
      await onCreateAnnotation("highlight", selectionMenu.boxes, selectionColor, selectionMenu.page, "", selectionMenu.anchor);
      window.getSelection()?.removeAllRanges(); setSelectionMenu(null); setAnnounce("Highlight saved");
      scrollRef.current?.focus({ preventScroll: true });
    } catch (error) { setAnnotationError(error instanceof Error ? error.message : "Could not save highlight. Please retry."); }
    finally { setAnnotationBusy(false); }
  }

  // ——— Suggestions: hit-test clicks instead of blocking selection with overlays ———
  const suggestionBoxesByPage = useMemo(() => {
    const map = new Map<number, ReturnType<typeof visibleSuggestionBoxes>>();
    const pages = new Set(suggestions.map((item) => item.src_page));
    for (const page of pages) map.set(page, visibleSuggestionBoxes(suggestions, page));
    return map;
  }, [suggestions]);
  const suggestionByID = useMemo(() => new Map(suggestions.map((item) => [item.id, item])), [suggestions]);

  function openPopover(suggestionId: string, page: number) {
    const boxes = (suggestionBoxesByPage.get(page) || []).filter((box) => box.suggestionID === suggestionId).map((box) => box.bbox);
    if (!boxes.length) return;
    const height = layoutRef.current.heights[page - 1] || 1;
    const width = layoutRef.current.widths[page - 1] || 1;
    const bottom = Math.max(...boxes.map((box) => box.y + box.h));
    const top = Math.min(...boxes.map((box) => box.y));
    const left = Math.min(...boxes.map((box) => box.x));
    const above = bottom * height + 230 > height && top * height > 230;
    setPopoverError("");
    setSelectionMenu(null);
    setPopover({
      suggestionId,
      page,
      x: Math.max(8, Math.min(left * width, width - 300)),
      y: above ? top * height - 8 : bottom * height + 8,
      above,
    });
  }

  function onPageClick(event: React.MouseEvent<HTMLDivElement>, page: number) {
    if (!suggestionsOn) return;
    const selection = window.getSelection();
    if (selection && !selection.isCollapsed) return;
    if ((event.target as HTMLElement).closest(".rd-popover, .rd-selection-menu, a")) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const x = (event.clientX - rect.left) / rect.width;
    const y = (event.clientY - rect.top) / rect.height;
    const hit = (suggestionBoxesByPage.get(page) || []).find(({ bbox }) => x >= bbox.x && x <= bbox.x + bbox.w && y >= bbox.y - 0.004 && y <= bbox.y + bbox.h + 0.004);
    if (hit) openPopover(hit.suggestionID, page);
    else setPopover(null);
  }

  async function respondFromPopover(action: "rejected") {
    const suggestion = popover && suggestionByID.get(popover.suggestionId);
    if (!suggestion || popoverBusy) return;
    setPopoverBusy(true);
    setPopoverError("");
    const saved = await onRespondSuggestion(suggestion.id, action);
    setPopoverBusy(false);
    if (saved) setPopover(null);
    else setPopoverError("Could not save this response. Please retry.");
  }

  // ——— Render ———
  const [first, last] = range;
  const mounted = (page: number) => page >= first && page <= last;
  const zoomLabel = zoom === "fit-width" ? "Fit width" : zoom === "fit-page" ? "Fit page" : `${Math.round(scale * 100)}%`;
  const zoomOptions: Array<[string, ZoomMode]> = [["Fit width", "fit-width"], ["Fit page", "fit-page"], ["50%", 0.5], ["75%", 0.75], ["100%", 1], ["125%", 1.25], ["150%", 1.5], ["200%", 2], ["300%", 3]];
  const zoomValue = typeof zoom === "number" ? String(zoom) : zoom;
  const innerWidth = Math.max(viewport.width, layout.maxWidth + PADDING * 2);
  // Transient UI belongs to a page; it disappears once that page leaves the view.
  const nearby = (page: number) => Math.abs(page - currentPage) <= 1;
  const popoverSuggestion = popover && nearby(popover.page) ? suggestionByID.get(popover.suggestionId) : undefined;
  const menuVisible = selectionMenu && nearby(selectionMenu.page) ? selectionMenu : null;

  return (
    <div ref={rootRef} className="rd-viewer" data-testid="pdf-viewer">
      <div className="rd-toolbar doc-toolbar" role="toolbar" aria-label="Reader controls">
        {toolbarStart}
        <div className="rd-grp">
          <button type="button" className={`rd-icon ${showThumbnails ? "on" : ""}`} aria-pressed={showThumbnails} aria-label="Page thumbnails" title="Page thumbnails" onClick={onToggleThumbnails}>
            <Icon name="doc" size={14} />
          </button>
        </div>
        <div className="rd-grp rd-pager">
          <button type="button" className="rd-icon" aria-label="Previous page" title="Previous page (←, PgUp)" disabled={currentPage <= 1} onClick={() => goToPage(currentPage - 1)}>
            <Icon name="chevron_left" size={14} />
          </button>
          <form
            className="rd-page-form"
            onSubmit={(event) => {
              event.preventDefault();
              const value = Number(pageInput);
              if (Number.isFinite(value) && value >= 1) goToPage(value);
              setPageInput(null);
              scrollRef.current?.focus({ preventScroll: true });
            }}
          >
            <input
              id={`rd-page-input-${docId}`}
              className="rd-page-input"
              aria-label={`Page number, 1 to ${numPages || "?"}`}
              inputMode="numeric"
              value={pageInput ?? String(currentPage)}
              onChange={(event) => setPageInput(event.target.value.replace(/[^\d]/g, "").slice(0, 5))}
              onFocus={(event) => {
                setPageInput(String(currentPage));
                event.currentTarget.select();
              }}
              onBlur={() => setPageInput(null)}
            />
            <span className="rd-page-total" aria-hidden="true">/ {numPages || "?"}</span>
            {/* Stable text hook for e2e checks and assistive tech: "3 / 16". */}
            <span className="page-indicator ui-visually-hidden">{currentPage} / {numPages || "?"}</span>
          </form>
          <button type="button" className="rd-icon" aria-label="Next page" title="Next page (→, PgDn)" disabled={!numPages || currentPage >= numPages} onClick={() => goToPage(currentPage + 1)}>
            <Icon name="chevron_right" size={14} />
          </button>
        </div>
        <div className="rd-grp rd-zoom">
          <button type="button" className="rd-icon" aria-label="Zoom out" title="Zoom out (Ctrl −)" onClick={() => zoomTo(stepZoom(scale, -1))}><Icon name="zoom_out" size={14} /></button>
          <select aria-label="Zoom" value={zoomOptions.some(([, value]) => String(value) === zoomValue) ? zoomValue : "custom"} onChange={(event) => {
            const value = event.target.value;
            zoomTo(value === "fit-width" || value === "fit-page" ? value : Number(value));
          }}>
            {!zoomOptions.some(([, value]) => String(value) === zoomValue) && <option value="custom">{zoomLabel}</option>}
            {zoomOptions.map(([label, value]) => <option key={label} value={String(value)}>{label}</option>)}
          </select>
          <button type="button" className="rd-icon" aria-label="Zoom in" title="Zoom in (Ctrl +)" onClick={() => zoomTo(stepZoom(scale, 1))}><Icon name="zoom_in" size={14} /></button>
        </div>
        <div className="rd-grp">
          <button type="button" className={`rd-icon ${findOpen ? "on" : ""}`} aria-label="Find in document" title="Find in document (Ctrl F)" aria-expanded={findOpen} onClick={() => {
            if (findOpen) closeFind();
            else { setFindOpen(true); requestAnimationFrame(() => findInputRef.current?.focus()); }
          }}>
            <Icon name="search" size={14} />
          </button>
        </div>
        {toolbarExtras}
        <button type="button" aria-label="My highlights" aria-expanded={highlightsOpen} onClick={() => setHighlightsOpen(value => !value)}>Highlights ({annotations.length})</button>
        <div className="rd-spacer" />
        {toolbarEnd}
      </div>

      {findOpen && (
        <div className="rd-findbar" role="search">
          <Icon name="search" size={13} />
          <input
            ref={findInputRef}
            aria-label="Find in document"
            placeholder="Find in document"
            value={findQuery}
            onChange={(event) => {
              setFindQuery(event.target.value);
              if (event.target.value.trim().length < 2) {
                setFindMatches([]);
                setFindIndex(-1);
                setFindActive("");
              }
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter") { event.preventDefault(); stepFind(event.shiftKey ? -1 : 1); }
              if (event.key === "Escape") { event.preventDefault(); closeFind(); }
            }}
          />
          <span className="rd-find-count" aria-live="polite">
            {findBusy ? "Searching…" : findQuery.trim().length < 2 ? "" : findMatches.length ? `${findIndex + 1} of ${findMatches.length}` : "No matches"}
          </span>
          <button type="button" className="rd-icon" aria-label="Previous match" disabled={!findMatches.length} onClick={() => stepFind(-1)}><Icon name="chevron_left" size={13} /></button>
          <button type="button" className="rd-icon" aria-label="Next match" disabled={!findMatches.length} onClick={() => stepFind(1)}><Icon name="chevron_right" size={13} /></button>
          <button type="button" className="rd-icon" aria-label="Close find" onClick={closeFind}><Icon name="x" size={13} /></button>
        </div>
      )}

      {editor && <AnnotationEditor key={editor.annotation?.id || "new-note"}
        title={editor.annotation ? "Edit highlight" : "Add note"}
        quote={editor.annotation?.anchor?.exact || editor.selection?.quote}
        color={editor.annotation?.color || selectionColor} comment={editor.annotation?.comment || ""}
        onCancel={() => setEditor(null)}
        onSave={async (color, comment) => {
          if (editor.annotation && onUpdateAnnotation) await onUpdateAnnotation(editor.annotation, color, comment);
          else if (editor.selection?.anchor && onCreateAnnotation) {
            if (editor.selection.anchor.source_hash !== sourceHash) throw new Error("Source changed. Select the passage again.");
            await onCreateAnnotation("note", editor.selection.boxes, color, editor.selection.page, comment, editor.selection.anchor);
          } else throw new Error("Annotation editing is unavailable.");
          setEditor(null); setSelectionMenu(null); window.getSelection()?.removeAllRanges(); setAnnounce("Annotation saved");
        }} />}
      {highlightsOpen && <HighlightsList annotations={annotations} sourceHash={sourceHash}
        onJump={a => {
          if (a.anchor && a.anchor.source_hash !== sourceHash) return;
          pendingFlashRef.current = { id: Date.now(), page: a.page, quote: a.anchor?.exact, anchor: a.anchor || undefined, bboxes: a.bbox };
          goToPage(a.page); setHighlightsOpen(false); setTextReady(ready => ({...ready}));
          scrollRef.current?.focus({ preventScroll: true });
        }}
        onEdit={a => setEditor({ annotation: a })}
        onDelete={async a => { if (!onDeleteAnnotation) throw new Error("Deleting is unavailable."); await onDeleteAnnotation(a); }} />}
      <div className="rd-body">
        <Document
          file={`/api/documents/${docId}/pdf`}
          onLoadSuccess={onDocumentLoad}
          loading={<div className="reader-empty">Loading document…</div>}
          error={<div className="reader-empty"><strong>Document unavailable</strong><span>Upload or reprocess the PDF to continue.</span></div>}
          className="rd-document"
        >
          {showThumbnails && numPages > 0 && (
            <nav className="rd-thumbs" aria-label="Page thumbnails">
              {Array.from({ length: numPages }, (_, index) => (
                <LazyThumb key={index} page={index + 1} active={index + 1 === currentPage} onPick={goToPage} />
              ))}
            </nav>
          )}
          <div
            ref={attachScroll}
            className="rd-scroll"
            tabIndex={0}
            aria-label="Document pages"
            onScroll={onScroll}
          >
            <div className="rd-column" style={{ height: layout.totalHeight || "100%", width: innerWidth || "100%" }}>
              {layout.tops.map((top, index) => {
                const page = index + 1;
                const width = layout.widths[index];
                const height = layout.heights[index];
                const style = { top, height, width, left: Math.max(PADDING, (innerWidth - width) / 2) };
                if (!mounted(page)) {
                  return <div key={page} className="rd-page rd-page-placeholder" data-page={page} style={style} aria-hidden="true"><span>{page}</span></div>;
                }
                const userMarks = annotationsOn ? annotations.filter((annotation) => annotation.page === page) : [];
                const pageSuggestions = suggestionsOn ? suggestionBoxesByPage.get(page) || [] : [];
                return (
                  <div key={page} className="rd-page pdf-page-container" data-page={page} style={style} role="group" aria-label={`Page ${page} of ${numPages}`} onClick={(event) => onPageClick(event, page)}>
                    <PdfPage page={page} scale={scale} textRenderer={findActive ? findRenderer : undefined} onTextReady={onTextReady} />
                    <div className="rd-marks" aria-hidden={!pageSuggestions.length && !userMarks.length}>
                      {pageSuggestions.map(({ suggestionID, bbox }, markIndex) => {
                        const suggestion = suggestionByID.get(suggestionID);
                        if (!suggestion) return null;
                        return (
                          <div
                            key={`s-${suggestionID}-${markIndex}`}
                            className={`rd-mark rd-evidence ${suggestion.status}`}
                            style={{ left: `${bbox.x * 100}%`, top: `${bbox.y * 100}%`, width: `${bbox.w * 100}%`, height: `${bbox.h * 100}%` }}
                            role="button"
                            tabIndex={0}
                            aria-label={`SELAR suggested passage: possible ${suggestion.relation.replaceAll("_", " ")} connection to ${suggestion.tgt_doc}, page ${suggestion.tgt_page}`}
                            onKeyDown={(event) => {
                              if (event.key === "Enter" || event.key === " ") { event.preventDefault(); openPopover(suggestionID, page); }
                            }}
                          />
                        );
                      })}
                      {userMarks.flatMap((annotation) => (markBoxes[annotation.id] || []).map((bbox, markIndex) => (
                        <div
                          key={`a-${annotation.id}-${markIndex}`}
                          className={`rd-mark rd-user ${annotation.type} c-${annotation.color}`}
                          style={{ left: `${bbox.x * 100}%`, top: `${bbox.y * 100}%`, width: `${bbox.w * 100}%`, height: `${bbox.h * 100}%` }}
                          title={annotation.comment || undefined}
                        />
                      )))}
                      {flash?.page === page && flash.boxes.map((bbox, markIndex) => (
                        <div key={`f-${flash.key}-${markIndex}`} className="rd-mark rd-flash" style={{ left: `${bbox.x * 100}%`, top: `${bbox.y * 100}%`, width: `${bbox.w * 100}%`, height: `${bbox.h * 100}%` }} />
                      ))}
                    </div>

                    {menuVisible?.page === page && (
                      <div className="rd-selection-menu" role="toolbar" aria-label="Selection actions" style={{ left: menuVisible.x, top: menuVisible.y }}>
                        <select aria-label="Selection highlight color" value={selectionColor} onChange={e => setSelectionColor(e.target.value as Annotation["color"])} disabled={annotationBusy}>{annotationColors.map(color => <option key={color} value={color}>{color}</option>)}</select>
                        <button type="button" disabled={!menuVisible.anchor || annotationBusy} onClick={createFromSelection}><Icon name="highlight" size={12} /> {annotationBusy ? "Saving…" : "Highlight"}</button>
                        <button type="button" disabled={!menuVisible.anchor || annotationBusy} onClick={() => setEditor({ selection: menuVisible })}><Icon name="note" size={12} /> Add note</button>
                        <button type="button" onClick={async () => { try { await navigator.clipboard.writeText(menuVisible.quote); setAnnounce("Quote copied"); } catch { setAnnotationError("Could not copy. Use your browser's copy command."); } }}>Copy</button>
                        <button type="button" aria-label="Close selection actions" disabled={annotationBusy} onClick={() => { setSelectionMenu(null); scrollRef.current?.focus({preventScroll:true}); }}>×</button>
                        {!menuVisible.anchor && <span>Source not ready, or selection outside text. Copy is still available.</span>}
                        {annotationError && <span role="alert">{annotationError}</span>}
                      </div>
                    )}

                    {popover?.page === page && popoverSuggestion && (
                      <div
                        className={`rd-popover ${popover.above ? "above" : ""}`}
                        style={{ left: popover.x, top: popover.y }}
                        role="dialog"
                        aria-label="Suggested connection"
                        onClick={(event) => event.stopPropagation()}
                      >
                        <div className="rd-popover-head">
                          <span>SELAR suggestion · {popoverSuggestion.relation.replaceAll("_", " ")}</span>
                          <button type="button" aria-label="Close suggestion" onClick={() => setPopover(null)}><Icon name="x" size={12} /></button>
                        </div>
                        <h3>{popoverSuggestion.tgt_doc} · page {popoverSuggestion.tgt_page}</h3>
                        <p>{popoverSuggestion.summary || popoverSuggestion.tgt_text.slice(0, 260)}</p>
                        <p className="rd-popover-note">Similarity only: these passages use similar wording. Compare them as reading material, not as an established relationship.</p>
                        {popoverError && <div className="rd-popover-error" role="alert">{popoverError}</div>}
                        <div className="rd-popover-actions">
                          <button type="button" className="rd-btn" disabled={!popoverSuggestion.tgt_document_id || popoverBusy} onClick={() => onOpenSuggestionTarget(popoverSuggestion)}>Open connected passage</button>
                          {popoverSuggestion.status === "pending" ? (
                            <button type="button" className="rd-btn" disabled={popoverBusy} onClick={() => respondFromPopover("rejected")}><Icon name="x" size={11} /> Dismiss</button>
                          ) : <span className={`rd-popover-state ${popoverSuggestion.status}`}>{popoverSuggestion.status === "rejected" ? "dismissed" : popoverSuggestion.status}</span>}
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </Document>
      </div>
      <div className="ui-visually-hidden" aria-live="polite">{announce}</div>
    </div>
  );
}

// react-pdf re-renders a text layer whenever its callbacks change identity,
// so each page owns a stable callback and only re-renders on real changes.
const PdfPage = memo(function PdfPage({ page, scale, textRenderer, onTextReady }: {
  page: number;
  scale: number;
  textRenderer?: (item: { str: string }) => string;
  onTextReady: (page: number) => void;
}) {
  const onSuccess = useCallback(() => onTextReady(page), [onTextReady, page]);
  return (
    <Page
      pageNumber={page}
      scale={scale}
      renderTextLayer
      renderAnnotationLayer
      customTextRenderer={textRenderer}
      onRenderTextLayerSuccess={onSuccess}
      loading={<div className="rd-page-loading" aria-hidden="true" />}
    />
  );
});

function LazyThumb({ page, active, onPick }: { page: number; active: boolean; onPick: (page: number) => void }) {
  const ref = useRef<HTMLButtonElement>(null);
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) setVisible(true);
    }, { rootMargin: "400px 0px" });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (active) ref.current?.scrollIntoView({ block: "nearest" });
  }, [active]);
  return (
    <button ref={ref} type="button" className={`rd-thumb ${active ? "on" : ""}`} aria-label={`Go to page ${page}`} aria-current={active ? "page" : undefined} onClick={() => onPick(page)}>
      <span className="rd-thumb-img">{visible ? <Page pageNumber={page} width={96} renderTextLayer={false} renderAnnotationLayer={false} loading={null} /> : null}</span>
      <span className="rd-thumb-num">{page}</span>
    </button>
  );
}

