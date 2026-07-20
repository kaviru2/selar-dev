"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Icon } from "@/components/ui/Icon";

export function RetryIngestionButton({ documentId }: { documentId: string }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function retry(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    event.stopPropagation();
    setBusy(true);
    setError("");
    try {
      const response = await fetch(`/api/documents/${documentId}/retry`, { method: "POST" });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(payload.error || "Unable to retry ingestion");
      router.refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to retry ingestion");
    } finally {
      setBusy(false);
    }
  }

  return <span className="retry-ingestion-wrap">
    <button className="btn retry-ingestion" disabled={busy} onClick={retry}>
      <Icon name={busy ? "spinner" : "refresh"} size={11} className={busy ? "animate-spin" : undefined} />
      {busy ? "Retrying…" : "Retry"}
    </button>
    {error && <small className="retry-ingestion-error">{error}</small>}
  </span>;
}
