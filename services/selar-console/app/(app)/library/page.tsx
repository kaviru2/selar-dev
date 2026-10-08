// page.tsx — Library: the signed-in home.
// Fetches document stats and the document list from the Go API.

import { Icon } from "@/components/ui/Icon";
import { getDocuments, getDocumentStats, type Document } from "@/lib/api";
import { getAuthToken } from "@/lib/auth";
import Link from "next/link";
import { AddContentButton } from "@/components/AddContentButton";
import { SourcesPanel } from "@/components/SourcesPanel";
import { DeleteDocButton } from "@/components/DeleteDocButton";
import { ProcessingRefresh } from "@/components/ProcessingRefresh";
import { RetryIngestionButton } from "@/components/RetryIngestionButton";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/Card";

function formatReadingTime(min: number) {
  const h = Math.floor(min / 60);
  const m = Math.round(min % 60);
  return h ? `${h}h ${m}m` : `${m}m`;
}

function StatusCell({ d }: { d: Document }) {
  if (d.status === "ready") {
    return <span className="lib-status lib-status--ready"><span className="lib-dot" aria-hidden="true" />Ready</span>;
  }
  if (d.status === "processing") {
    const pct = Math.round((d.progress ?? 0) * 100);
    const queued = d.ingestion_status === "queued";
    return (
      <span className="lib-status lib-status--processing">
        <span
          className="lib-progress"
          role="progressbar"
          aria-label={`Processing ${d.title}`}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={queued ? undefined : pct}
        >
          <span style={{ width: `${queued ? 8 : Math.max(pct, 4)}%` }} />
        </span>
        <span className="lib-status-text">{queued ? "Queued" : `${pct}%`}</span>
      </span>
    );
  }
  return (
    <span className="lib-status lib-status--failed" title={d.ingestion_error || "Processing failed"}>
      <span className="lib-status-text">
        Couldn&apos;t process
        {d.ingestion_error && <span className="lib-error-detail">{d.ingestion_error.slice(0, 80)}</span>}
      </span>
      <RetryIngestionButton documentId={d.id} />
    </span>
  );
}

export default async function LibraryPage() {
  const token = await getAuthToken();

  if (!token) {
    return <div className="app-page">Unauthorized. Please log in.</div>;
  }

  let docs: Document[] = [];
  let stats = { total_documents: 0, total_chunks: 0, confirmed_links: 0, reading_time_min: 0 };
  let loadFailed = false;

  try {
    const [fetchedDocs, fetchedStats] = await Promise.all([getDocuments(token), getDocumentStats(token)]);
    docs = fetchedDocs;
    stats = fetchedStats;
  } catch (error) {
    loadFailed = true;
    console.error("Failed to fetch library data", error);
  }

  const STATS = [
    { label: "Readings", value: stats.total_documents.toString(), icon: "book" as const, tone: "green" },
    { label: "Links you kept", value: stats.confirmed_links.toString(), icon: "link" as const, tone: "warm" },
    { label: "Reading time", value: formatReadingTime(stats.reading_time_min), icon: "clock" as const, tone: "sky" },
    { label: "Passages indexed", value: stats.total_chunks.toString(), icon: "doc" as const, tone: "amber" },
  ];

  const processing = docs.some((d) => d.status === "processing");

  return (
    <div className="app-page library">
      <ProcessingRefresh active={processing} />
      <div className="app-page-inner">
        <PageHeader
          eyebrow="Your library"
          title="Library"
          description={
            docs.length
              ? "Open a reading to see what it might connect to. You decide which links are worth keeping."
              : "Add a couple of readings from the same course to get started."
          }
          actions={<AddContentButton />}
        />

        <ul className="lib-stats" aria-label="Library summary">
          {STATS.map((s) => (
            <li key={s.label} className={`lib-stat lib-stat--${s.tone}`}>
              <span className="lib-stat-icon" aria-hidden="true"><Icon name={s.icon} size={16} /></span>
              <span className="lib-stat-val">{s.value}</span>
              <span className="lib-stat-lbl">{s.label}</span>
            </li>
          ))}
        </ul>

        {loadFailed && (
          <div className="ui-notice lib-notice" role="status">
            <Icon name="info" size={16} />
            <span><strong>We couldn&apos;t load your library just now.</strong> Your readings are safe. Try refreshing in a moment.</span>
          </div>
        )}

        <section className="lib-docs" aria-labelledby="lib-docs-title">
          <h2 id="lib-docs-title" className="ui-visually-hidden">Readings</h2>
          {docs.length === 0 ? (
            <EmptyState
              mascot="empty"
              title={loadFailed ? "Nothing to show yet" : "Your shelf is empty, for now"}
              actions={loadFailed ? undefined : <AddContentButton />}
            >
              Upload a PDF, paste some notes, or save an article. Linny will look for passages that might link to
              something you read before.
            </EmptyState>
          ) : (
            <div className="lib-table" role="table" aria-label="Readings">
              <div className="lib-row lib-row--head" role="row">
                <span role="columnheader">Title</span>
                <span role="columnheader">Authors</span>
                <span role="columnheader">Year</span>
                <span role="columnheader">Pages</span>
                <span role="columnheader">Status</span>
                <span role="columnheader"><span className="ui-visually-hidden">Actions</span></span>
              </div>
              {docs.map((d) => (
                <div key={d.id} className="lib-row" role="row">
                  <span role="cell" className="lib-title">
                    <span className="lib-doc-icon" aria-hidden="true"><Icon name={d.source_type === "pdf" ? "doc" : "note"} size={15} /></span>
                    <Link href={`/reader?docId=${d.id}`} className="lib-title-link">{d.title}</Link>
                  </span>
                  <span role="cell" className="lib-muted lib-authors">{d.authors || "Unknown"}</span>
                  <span role="cell" className="lib-num">{d.year || "—"}</span>
                  <span role="cell" className="lib-num">{d.source_type === "pdf" && d.page_count ? d.page_count : "—"}</span>
                  <span role="cell" className="lib-status-cell"><StatusCell d={d} /></span>
                  <span role="cell" className="lib-actions"><DeleteDocButton docId={d.id} /></span>
                </div>
              ))}
            </div>
          )}
        </section>

        <SourcesPanel />
      </div>
    </div>
  );
}
