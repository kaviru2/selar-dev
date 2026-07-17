// page.tsx — Library dashboard view.
// Fetches real document stats and document list from the Go API.

import { Icon } from "@/components/ui/Icon";
import { getDocuments, getDocumentStats, type Document } from "@/lib/api";
import { getAuthToken } from "@/lib/auth";
import Link from "next/link";
import { AddContentButton } from "@/components/AddContentButton";
import { SourcesPanel } from "@/components/SourcesPanel";
import { DeleteDocButton } from "@/components/DeleteDocButton";
import { ProcessingRefresh } from "@/components/ProcessingRefresh";

export default async function LibraryPage() {
  const token = await getAuthToken();
  
  if (!token) {
    return <div>Unauthorized. Please log in.</div>;
  }

  let docs: Document[] = [];
  let stats = { total_documents: 0, total_chunks: 0, confirmed_links: 0, reading_time_min: 0 };
  
  try {
    const [fetchedDocs, fetchedStats] = await Promise.all([
      getDocuments(token),
      getDocumentStats(token)
    ]);
    docs = fetchedDocs;
    stats = fetchedStats;
  } catch (error) {
    console.error("Failed to fetch library data", error);
  }

  const STATS = [
    { label: "Documents", value: stats.total_documents.toString(), delta: "" },
    { label: "Confirmed links", value: stats.confirmed_links.toString(), delta: "" },
    { label: "Reading time", value: `${Math.round(stats.reading_time_min / 60)}h ${Math.round(stats.reading_time_min % 60)}m`, delta: "", muted: true },
    { label: "Chunks extracted", value: stats.total_chunks.toString(), delta: "" },
  ];

  return (
    <div className="library">
      <ProcessingRefresh active={docs.some((document) => document.status === "processing")} />
      <div className="head">
        <div>
          <h1>Library</h1>
          <div className="sub">{stats.total_documents} documents · {stats.total_chunks} chunks · {stats.confirmed_links} confirmed links</div>
        </div>
        <div style={{ flex: 1 }} />
        <button className="btn">
          <Icon name="filter" size={12} /> Filter
        </button>
        <button className="btn">
          <Icon name="drive" size={12} /> From Drive
        </button>
        <AddContentButton />
      </div>

      <div className="stats">
        {STATS.map((s) => (
          <div key={s.label} className="stat">
            <div className="lbl">{s.label}</div>
            <div className="val">{s.value}</div>
            <div className="delta" style={s.muted ? { color: "var(--ink-4)" } : undefined}>
              {s.delta}
            </div>
          </div>
        ))}
      </div>

      <div className="doc-table">
        <div className="doc-row head">
          <span />
          <span>Title</span>
          <span>Authors</span>
          <span>Year</span>
          <span>Pages</span>
          <span>Status</span>
          <span />
        </div>
        
        {docs.length === 0 ? (
          <div style={{ textAlign: "center", padding: "40px", color: "var(--ink-4)" }}>
            Your library is empty. Add an article, paste research notes, or upload a PDF to begin.
          </div>
        ) : docs.map((d) => (
          <Link key={d.id} href={`/reader?docId=${d.id}`} style={{ textDecoration: "none", color: "inherit", display: "contents" }}>
            <div className="doc-row" style={{ cursor: "pointer" }}>
              <span className={`dot ${d.status}`} />
              <span className="title">{d.title}</span>
              <span className="authors">{d.authors || "Unknown"}</span>
              <span className="mono">{d.year || "—"}</span>
              <span className="mono">{d.source_type === "pdf" && d.page_count ? `${d.page_count}p` : "—"}</span>
              <span>
                {d.status === "ready" ? (
                  <span className="mono" style={{ color: "var(--accent-2)" }}>ready</span>
                ) : d.status === "processing" ? (
                  <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
                    <div className="prog" style={{ height: 3, background: "var(--bg-3)", borderRadius: 2, overflow: "hidden", width: 80 }}>
                      <div className="fill" style={{ width: `${(d.progress ?? 0) * 100}%`, height: "100%", background: "var(--accent)" }} />
                    </div>
                    <span className="mono" style={{ fontSize: 10 }}>{Math.round((d.progress ?? 0) * 100)}%</span>
                  </span>
                ) : (
                  <span className="mono" style={{ color: "var(--ink-4)" }}>{d.status}</span>
                )}
              </span>
              <DeleteDocButton docId={d.id} />
            </div>
          </Link>
        ))}
      </div>
      <SourcesPanel />
    </div>
  );
}
