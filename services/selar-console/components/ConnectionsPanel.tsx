"use client";
// Guided Connections sidebar: one suggested link at a time, as
// notice → explain → compare & decide. Review semantics are unchanged: every
// decision is a revision-bound call to /api/mental-model-links/{id}/respond.
import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { track } from "@/lib/analytics";
import { Icon } from "@/components/ui/Icon";
import {
  clientFetch,
  type DocumentMentalModel,
  type MentalLinkReviewPreview,
  type MentalModelLink,
} from "@/lib/api";
import { readerWitnessURL, type ReviewAction } from "@/lib/reviewed-links";
import {
  DECISION_WORDS,
  keptLinks,
  linksToReview,
  locatorLabel,
  otherSide,
  overlapStrength,
  setAsideLinks,
  sharedIdea,
  whySuggested,
  type GuideStep,
} from "@/lib/connection-guide";

type Tab = "review" | "kept" | "about";

const NOT_REAL_REASONS = ["They only share a word", "Different meaning in each reading", "Not useful for my learning"];

export interface ConnectionsPanelProps {
  docId: string;
  links: MentalModelLink[];
  loading: boolean;
  mentalModel: DocumentMentalModel | null;
  /** Re-fetch links after a decision; must resolve to the fresh list. */
  reloadLinks: () => Promise<MentalModelLink[]>;
  /** Optional link to open first (e.g. from ?linkId=). */
  focusLinkId?: string | null;
  /** Similarity-only passage matches, shown under "About this reading". */
  passageSection?: ReactNode;
  /**
   * Optional in-place jump for "open ↗" on a compare quote. Return true when
   * handled (e.g. scrolled the open reader and flashed the passage); otherwise
   * the link navigates normally.
   */
  onOpenWitness?: OpenWitness;
}

export type OpenWitness = (docId: string, locator: { page?: number; block_index?: number } | undefined, quote: string) => boolean;

interface LastDecision {
  linkId: string;
  action: ReviewAction;
  otherTitle: string;
}

async function respond(id: string, body: Record<string, unknown>) {
  await clientFetch(`/api/mental-model-links/${id}/respond`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

function hasLocator(locator?: { page?: number; block_index?: number }) {
  return Boolean(locator && (Number.isInteger(locator.page) || Number.isInteger(locator.block_index)));
}

export function isReviewable(preview: MentalLinkReviewPreview | null): preview is MentalLinkReviewPreview {
  return Boolean(
    preview && preview.id && preview.source_document_id && preview.target_document_id &&
    preview.source_quote?.trim() && preview.target_quote?.trim() &&
    hasLocator(preview.source_locator) && hasLocator(preview.target_locator),
  );
}

export function ConnectionsPanel({ docId, links, loading, mentalModel, reloadLinks, focusLinkId, passageSection, onOpenWitness }: ConnectionsPanelProps) {
  const [chosenTab, setTab] = useState<Tab | null>(null);
  const [skipped, setSkipped] = useState<string[]>([]);
  const [last, setLast] = useState<LastDecision | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  const queue = useMemo(() => {
    const pending = linksToReview(links);
    const fresh = pending.filter((link) => !skipped.includes(link.id));
    const later = pending.filter((link) => skipped.includes(link.id));
    const ordered = [...fresh, ...later];
    if (focusLinkId) {
      const index = ordered.findIndex((link) => link.id === focusLinkId);
      if (index > 0) ordered.unshift(...ordered.splice(index, 1));
    }
    return ordered;
  }, [links, skipped, focusLinkId]);
  const kept = useMemo(() => keptLinks(links), [links]);
  const setAside = useMemo(() => setAsideLinks(links), [links]);
  const current = queue[0] ?? null;

  // The parent keys this panel by document, so per-document state resets on navigation.
  const tab: Tab = chosenTab ?? (focusLinkId && kept.some((link) => link.id === focusLinkId) ? "kept" : "review");

  const decide = useCallback(async (link: MentalModelLink, action: ReviewAction, revision: number, extra: Record<string, unknown> = {}) => {
    setError("");
    try {
      await respond(link.id, { action, revision, ...extra });
      await reloadLinks();
      const { otherTitle } = otherSide(link, docId);
      setLast({ linkId: link.id, action, otherTitle });
      setNotice(
        action === "confirmed" ? "Link kept. It now appears in your graph and can be cited in chat."
          : action === "relabeled" ? "Link kept with your relationship note."
          : action === "rejected" ? "Marked as not a real link. It will not appear in your graph."
          : action === "retracted" ? "Link removed from your kept links."
          : "Decision undone.",
      );
      return true;
    } catch (err) {
      setError(err instanceof Error ? `${err.message}. The link may have changed; it has been reloaded.` : "Could not save your decision.");
      await reloadLinks().catch(() => undefined);
      return false;
    }
  }, [docId, reloadLinks]);

  const undoLast = useCallback(async () => {
    if (!last) return;
    setError("");
    try {
      const preview = await clientFetch<MentalLinkReviewPreview>(`/api/mental-model-links/${last.linkId}/preview`);
      await respond(last.linkId, {
        action: "rolled_back", revision: preview.revision, target_revision: preview.revision - 1,
        reason: "Undone right after deciding",
      });
      await reloadLinks();
      setLast(null);
      setNotice("Decision undone. The link is back where it was.");
    } catch (err) {
      setError(err instanceof Error ? `Could not undo: ${err.message}` : "Could not undo.");
    }
  }, [last, reloadLinks]);

  return (
    <aside className="matches mental-panel cx-panel" aria-label="Connections">
      <div className="cx-head">
        <h2 className="cx-title">Connections</h2>
        <span className="cx-count">{loading ? "…" : `${queue.length} to review · ${kept.length} kept`}</span>
      </div>
      <div className="cx-tabs" role="tablist" aria-label="Connections view">
        <button role="tab" aria-selected={tab === "review"} className={tab === "review" ? "on" : ""} onClick={() => setTab("review")}>To review ({queue.length})</button>
        <button role="tab" aria-selected={tab === "kept"} className={tab === "kept" ? "on" : ""} onClick={() => setTab("kept")}>Kept links</button>
        <button role="tab" aria-selected={tab === "about"} className={tab === "about" ? "on" : ""} onClick={() => setTab("about")}>About this reading</button>
      </div>

      <div className="matches-body cx-body">
        {notice && (
          <div className="cx-toast" role="status">
            <span>{notice}</span>
            {last && last.action !== "rolled_back" && <button type="button" onClick={undoLast}>Undo</button>}
            <button type="button" aria-label="Dismiss message" className="cx-x" onClick={() => { setNotice(""); setLast(null); }}><Icon name="x" size={11} /></button>
          </div>
        )}
        {error && <p role="alert" className="cx-error">{error}</p>}

        {tab === "review" && (
          loading ? <p className="cx-empty">Loading connections…</p>
            : current ? (
              <GuidedReview
                key={`${docId}:${current.id}`}
                docId={docId}
                link={current}
                onOpenWitness={onOpenWitness}
                position={1}
                total={queue.length}
                onDecide={decide}
                onLater={() => setSkipped((ids) => [...ids.filter((id) => id !== current.id), current.id])}
              />
            ) : (
              <div className="cx-empty">
                <Icon name="check" size={18} />
                <strong>{links.length ? "Nothing left to review" : "No suggested connections yet"}</strong>
                <span>{links.length
                  ? "Your kept links are in the Kept links tab and in the graph."
                  : "SELAR suggests a link only when this reading and another one both name the same concept in their own text. Similar passages are listed under About this reading; they are not links."}</span>
                {kept.length > 0 && <button type="button" className="cx-btn cx-sec" onClick={() => setTab("kept")}>See kept links</button>}
              </div>
            )
        )}

        {tab === "kept" && (
          <KeptList docId={docId} kept={kept} setAside={setAside} onDecide={decide} />
        )}

        {tab === "about" && (
          <div className="cx-about">
            {mentalModel ? <ReadingSummary model={mentalModel} /> : <p className="cx-empty">A summary appears once this reading has been processed.</p>}
            {passageSection}
          </div>
        )}
      </div>
    </aside>
  );
}

function Progress({ step }: { step: GuideStep }) {
  const index = step === "notice" ? 1 : step === "explain" ? 2 : 3;
  return (
    <div className="cx-prog" aria-label={`Step ${index} of 3`}>
      {[1, 2, 3].map((n) => <div key={n} className={n <= index ? "d" : ""} />)}
    </div>
  );
}

function GuidedReview({ docId, link, position, total, onDecide, onLater, onOpenWitness }: {
  docId: string;
  onOpenWitness?: OpenWitness;
  link: MentalModelLink;
  position: number;
  total: number;
  onDecide: (link: MentalModelLink, action: ReviewAction, revision: number, extra?: Record<string, unknown>) => Promise<boolean>;
  onLater: () => void;
}) {
  const [step, setStep] = useState<GuideStep>("notice");
  const [preview, setPreview] = useState<MentalLinkReviewPreview | null>(null);
  const [previewError, setPreviewError] = useState("");
  const [explanation, setExplanation] = useState("");
  const [recallOn, setRecallOn] = useState(false);
  const [recall, setRecall] = useState("");
  const [choice, setChoice] = useState<"" | "relabel" | "reject">("");
  const [label, setLabel] = useState("");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const { thisTitle, otherTitle } = otherSide(link, docId);
  const idea = sharedIdea(link);

  useEffect(() => {
    let active = true;
    clientFetch<MentalLinkReviewPreview>(`/api/mental-model-links/${link.id}/preview`)
      .then((data) => { if (active) setPreview(data); })
      .catch((err) => { if (active) setPreviewError(err instanceof Error ? err.message : "Evidence unavailable"); });
    return () => { active = false; };
  }, [link.id]);

  const moved = useRef(false);
  useEffect(() => {
    if (moved.current) headingRef.current?.focus();
    moved.current = true;
  }, [step]);

  // First-party usage analytics (no-op without consent; lengths only, never text).
  useEffect(() => {
    track("suggestion_shown", { kind: "mental_link", link_id: link.id, position });
  }, [link.id, position]);
  const openExplain = () => {
    track("suggestion_opened", { kind: "mental_link", link_id: link.id });
    setStep("explain");
  };
  const openCompare = () => {
    track("explain_submitted", { link_id: link.id, length: explanation.trim().length, recall_length: recall.trim().length, recall_on: recallOn });
    track("compare_viewed", { kind: "mental_link", link_id: link.id });
    setStep("compare");
  };

  const reviewable = isReviewable(preview);
  const act = async (action: ReviewAction, extra: Record<string, unknown> = {}) => {
    if (!reviewable || busy) return;
    setBusy(true);
    const ok = await onDecide(link, action, preview.revision, extra);
    if (!ok) setBusy(false);
  };

  return (
    <section className="cx-card" aria-label="Review a suggested connection">
      <div className="cx-meta">
        <span>{total > 1 ? `Suggestion ${position} of ${total}` : "One suggestion"}</span>
        <span>Step {step === "notice" ? 1 : step === "explain" ? 2 : 3} of 3</span>
      </div>
      <Progress step={step} />

      {step === "notice" && (
        <>
          <h3 ref={headingRef} tabIndex={-1} className="cx-h">A possible connection</h3>
          <div className="cx-lbl">This reading</div>
          <div className="cx-doc">{thisTitle}</div>
          <div className="cx-arrow" aria-hidden>may connect to ↓</div>
          <div className="cx-lbl">Your earlier reading</div>
          <div className="cx-doc">{otherTitle}</div>
          <p className="cx-why"><strong>Why SELAR suggests this:</strong> {whySuggested(link)}</p>
          <div className="cx-chips">
            {idea && <span className="cx-chip">Shared idea: {idea}</span>}
            <span className="cx-chip g">{overlapStrength(link.confidence)}</span>
          </div>
          <p className="cx-note">This is a suggestion, not a fact. The two readings may only share a topic. You decide whether the link is real.</p>
          {previewError && <p role="alert" className="cx-error">The source passages could not be loaded ({previewError}). This link cannot be reviewed right now.</p>}
          {preview && !reviewable && <p role="alert" className="cx-error">Two exact source passages and their locations are unavailable, so this link cannot be reviewed.</p>}
          <button type="button" className="cx-btn cx-pri" disabled={!reviewable} onClick={openExplain}>Think about this link →</button>
          <button type="button" className="cx-btn cx-ghost" onClick={onLater}>Skip for now</button>
        </>
      )}

      {step === "explain" && (
        <>
          <h3 ref={headingRef} tabIndex={-1} className="cx-h">Explain it in your own words</h3>
          <label className="cx-q" htmlFor={`explain-${link.id}`}>How does this reading relate to <em>{otherTitle}</em>?</label>
          <textarea
            id={`explain-${link.id}`}
            className="cx-ta"
            aria-label="Your explanation of how the readings connect"
            placeholder="Write what you think the connection is before looking at the passages. You can leave this blank."
            value={explanation}
            onChange={(event) => setExplanation(event.target.value)}
          />
          <label className="cx-toggle">
            <input type="checkbox" checked={recallOn} onChange={(event) => setRecallOn(event.target.checked)} />
            <span>Also recall what <em>{otherTitle}</em> said, without looking (optional)</span>
          </label>
          {recallOn && (
            <textarea
              className="cx-ta cx-ta-sm"
              aria-label="Your recall of the earlier reading"
              placeholder="What do you remember the earlier reading saying about this?"
              autoComplete="off"
              value={recall}
              onChange={(event) => setRecall(event.target.value)}
            />
          )}
          <p className="cx-note">Your writing is just for you. It is not graded, not saved and not sent anywhere.</p>
          <button type="button" className="cx-btn cx-pri" onClick={openCompare}>Show me the passages →</button>
          <button type="button" className="cx-btn cx-ghost" onClick={() => setStep("notice")}>← Back</button>
        </>
      )}

      {step === "compare" && reviewable && (
        <>
          <h3 ref={headingRef} tabIndex={-1} className="cx-h">Compare with the sources</h3>
          {(explanation.trim() || recall.trim()) && (
            <div className="cx-mine">
              {explanation.trim() && <><b>You wrote</b><p>{explanation}</p></>}
              {recall.trim() && <><b>You recalled</b><p>{recall}</p></>}
            </div>
          )}
          <div aria-label="Source passages for comparison">
            <Quote title={preview.source_document_title} quote={preview.source_quote!} where={locatorLabel(preview.source_locator)} href={readerWitnessURL(preview.source_document_id, preview.source_locator)} isThis={preview.source_document_id === docId} onOpen={onOpenWitness && (() => onOpenWitness(preview.source_document_id, preview.source_locator, preview.source_quote!))} />
            <Quote title={preview.target_document_title} quote={preview.target_quote!} where={locatorLabel(preview.target_locator)} href={readerWitnessURL(preview.target_document_id, preview.target_locator)} isThis={preview.target_document_id === docId} onOpen={onOpenWitness && (() => onOpenWitness(preview.target_document_id, preview.target_locator, preview.target_quote!))} />
          </div>
          <div className="cx-decide">
          <div className="cx-lbl cx-decide-lbl">Is this a real link?</div>
          <button type="button" className="cx-btn cx-pri" disabled={busy} onClick={() => act("confirmed")}>
            <Icon name="check" size={12} /> Yes, keep this link
          </button>
          <div className="cx-dec">
            <button type="button" className={`cx-btn cx-sec ${choice === "relabel" ? "on" : ""}`} aria-expanded={choice === "relabel"} disabled={busy} onClick={() => setChoice(choice === "relabel" ? "" : "relabel")}><Icon name="tag" size={11} /> Different relationship</button>
            <button type="button" className={`cx-btn cx-sec ${choice === "reject" ? "on" : ""}`} aria-expanded={choice === "reject"} disabled={busy} onClick={() => setChoice(choice === "reject" ? "" : "reject")}><Icon name="x" size={11} /> Not a real link</button>
          </div>
          {choice === "relabel" && (
            <div className="cx-sub">
              <label htmlFor={`label-${link.id}`}>How are they related? (your own note)</label>
              <input id={`label-${link.id}`} value={label} maxLength={160} placeholder="e.g. builds on, gives an example of" onChange={(event) => setLabel(event.target.value)} />
              <button type="button" className="cx-btn cx-pri" disabled={busy || !label.trim()} onClick={() => act("relabeled", { label: label.trim() })}>Keep link with this note</button>
            </div>
          )}
          {choice === "reject" && (
            <div className="cx-sub">
              <span className="cx-sublbl">Why isn&apos;t it a real link?</span>
              <div className="cx-reasons">
                {NOT_REAL_REASONS.map((text) => (
                  <button key={text} type="button" className={`cx-reason ${reason === text ? "on" : ""}`} onClick={() => setReason(text)}>{text}</button>
                ))}
              </div>
              <label className="cx-sublbl" htmlFor={`reason-${link.id}`}>Or in your own words</label>
              <input id={`reason-${link.id}`} aria-label="Reason it is not a real link" value={reason} maxLength={500} placeholder="Why the link is not real" onChange={(event) => setReason(event.target.value)} />
              <button type="button" className="cx-btn cx-pri" disabled={busy || !reason.trim()} onClick={() => act("rejected", { reason: reason.trim() })}>Confirm: not a real link</button>
            </div>
          )}
          <div className="cx-row cx-row-split">
            <button type="button" className="cx-btn cx-ghost" onClick={() => setStep("explain")}>← Back</button>
            <button type="button" className="cx-btn cx-ghost" onClick={onLater}>Skip for now</button>
          </div>
          </div>
        </>
      )}
    </section>
  );
}

function Quote({ title, quote, where, href, isThis, onOpen }: { title: string; quote: string; where: string; href: string; isThis: boolean; onOpen?: () => boolean }) {
  return (
    <figure className="cx-quote">
      <figcaption>
        <span className="cx-qsrc">{isThis ? "This reading" : title}</span>
        {where && <span> · {where}</span>}
        <span> · </span><a href={href} onClick={(event) => { if (onOpen?.()) event.preventDefault(); }}>open ↗</a>
      </figcaption>
      <blockquote>“{quote}”</blockquote>
    </figure>
  );
}

function KeptList({ docId, kept, setAside, onDecide }: {
  docId: string;
  kept: MentalModelLink[];
  setAside: MentalModelLink[];
  onDecide: (link: MentalModelLink, action: ReviewAction, revision: number, extra?: Record<string, unknown>) => Promise<boolean>;
}) {
  if (!kept.length && !setAside.length) {
    return <div className="cx-empty"><Icon name="link" size={18} /><strong>No kept links yet</strong><span>Links you keep from the To review tab appear here, in the graph and in chat citations.</span></div>;
  }
  return (
    <div className="cx-kept">
      {kept.map((link) => <KeptItem key={link.id} docId={docId} link={link} onDecide={onDecide} />)}
      {setAside.length > 0 && (
        <details className="cx-aside">
          <summary>Marked as not real ({setAside.length})</summary>
          {setAside.map((link) => <KeptItem key={link.id} docId={docId} link={link} onDecide={onDecide} />)}
        </details>
      )}
    </div>
  );
}

function KeptItem({ docId, link, onDecide }: {
  docId: string;
  link: MentalModelLink;
  onDecide: (link: MentalModelLink, action: ReviewAction, revision: number, extra?: Record<string, unknown>) => Promise<boolean>;
}) {
  const [open, setOpen] = useState(false);
  const [preview, setPreview] = useState<MentalLinkReviewPreview | null>(null);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const { thisTitle, otherTitle, otherId } = otherSide(link, docId);
  const idea = sharedIdea(link);
  const isKept = link.status === "confirmed" || link.status === "relabeled";

  useEffect(() => {
    if (!open) return;
    let active = true;
    clientFetch<MentalLinkReviewPreview>(`/api/mental-model-links/${link.id}/preview`)
      .then((data) => { if (active) setPreview(data); })
      .catch(() => { if (active) setPreview(null); });
    return () => { active = false; };
  }, [open, link.id, link.revision]);

  const run = async (action: ReviewAction) => {
    if (!preview || busy || !reason.trim()) return;
    setBusy(true);
    const extra: Record<string, unknown> = { reason: reason.trim() };
    if (action === "rolled_back") extra.target_revision = preview.revision - 1;
    await onDecide(link, action, preview.revision, extra);
    setBusy(false);
  };

  return (
    <article className={`cx-item ${isKept ? "" : "aside"}`}>
      <div className="cx-item-t">↔ {otherTitle}</div>
      <div className="cx-item-from">with {thisTitle}</div>
      <div className="cx-item-m">
        {link.status === "relabeled" && link.user_label ? `Your note: ${link.user_label}` : idea ? `Shared idea: ${idea}` : "Shared concept"}
        {" · "}<Link href={`/reader?docId=${otherId}`}>open reading</Link>
        {isKept && <>{" · "}<Link href="/graph">view in graph</Link></>}
      </div>
      <button type="button" className="cx-link" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? "Hide details" : "Details and history"}</button>
      {open && (
        <div className="cx-sub">
          {preview ? (
            <>
              <div className="cx-sublbl">Decision history (oldest first)</div>
              <ol className="cx-hist" aria-label="Decision history">
                {preview.history.map((event) => (
                  <li key={event.revision}>{DECISION_WORDS[event.action] || event.action}{event.reason ? ` — “${event.reason}”` : ""}{event.occurred_at ? ` · ${new Date(event.occurred_at).toLocaleString(undefined, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}` : ""}</li>
                ))}
              </ol>
              <label className="cx-sublbl" htmlFor={`change-${link.id}`}>To change this decision, give a short reason</label>
              <input id={`change-${link.id}`} aria-label="Reason for the change" value={reason} maxLength={500} placeholder="e.g. I misread the passage" onChange={(event) => setReason(event.target.value)} />
              <div className="cx-row">
                {isKept && <button type="button" className="cx-btn cx-sec cx-danger" disabled={busy || !reason.trim()} onClick={() => run("retracted")}>Remove this link</button>}
                {preview.revision > 0 && <button type="button" className="cx-btn cx-sec" disabled={busy || !reason.trim()} onClick={() => run("rolled_back")}>Undo last decision</button>}
              </div>
            </>
          ) : <p className="cx-empty-sm">Loading history…</p>}
        </div>
      )}
    </article>
  );
}

function ReadingSummary({ model }: { model: DocumentMentalModel }) {
  return (
    <section className="cx-summary" aria-label="About this reading">
      <div className="cx-lbl">Main claim</div>
      <p>{model.main_claim}</p>
      {model.key_concepts.length > 0 && (
        <>
          <div className="cx-lbl">Key ideas</div>
          <div className="cx-chips">{model.key_concepts.map((concept) => <span key={concept} className="cx-chip g">{concept}</span>)}</div>
        </>
      )}
      {model.assumptions.length > 0 && (
        <details><summary>Assumptions ({model.assumptions.length})</summary><ul>{model.assumptions.map((item) => <li key={item}>{item}</li>)}</ul></details>
      )}
      {model.open_questions.length > 0 && (
        <details><summary>Open questions ({model.open_questions.length})</summary><ul>{model.open_questions.map((item) => <li key={item}>{item}</li>)}</ul></details>
      )}
      <p className="cx-note">Generated automatically from this reading; check it against the text.</p>
    </section>
  );
}
