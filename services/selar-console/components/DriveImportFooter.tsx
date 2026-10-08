// DriveImportFooter.tsx — sidebar footer for Google Drive.
// Replaces the old hard-coded "Drive · connected" label, which claimed a
// connection that never existed. SELAR keeps no standing Drive connection:
// each import asks Google for a short-lived drive.file token, the learner
// picks one PDF, and the API downloads it once. So the footer offers an
// action, or says plainly that the import isn't set up.

"use client";

import { useState } from "react";
import { Icon } from "@/components/ui/Icon";
import { DriveCancelled, driveImportEnabled, driveStatusLabel, importFromDrive } from "@/lib/google-drive";

type Status = { kind: "idle" } | { kind: "busy" } | { kind: "done"; name: string } | { kind: "error"; message: string };

export function DriveImportFooter({ onImported }: { onImported?: () => void }) {
  const enabled = driveImportEnabled();
  const [status, setStatus] = useState<Status>({ kind: "idle" });

  if (!enabled) {
    return (
      <div className="sb-foot" data-testid="drive-footer">
        <Icon name="drive" size={12} style={{ color: "var(--ink-4)" }} />
        <span style={{ fontSize: 11 }}>{driveStatusLabel(false)}</span>
      </div>
    );
  }

  const run = async () => {
    setStatus({ kind: "busy" });
    try {
      const name = await importFromDrive();
      if (name === null) {
        setStatus({ kind: "idle" });
        return;
      }
      setStatus({ kind: "done", name });
      onImported?.();
    } catch (err) {
      if (err instanceof DriveCancelled) {
        setStatus({ kind: "idle" });
        return;
      }
      setStatus({ kind: "error", message: err instanceof Error ? err.message : "Import from Google Drive failed" });
    }
  };

  return (
    <div className="sb-foot" data-testid="drive-footer" style={{ flexDirection: "column", alignItems: "stretch", gap: 4 }}>
      <button
        type="button"
        onClick={run}
        disabled={status.kind === "busy"}
        className="ui-btn ui-btn--ghost ui-btn--sm"
        style={{ justifyContent: "flex-start", paddingInline: 4 }}
      >
        <Icon name="drive" size={12} style={{ color: "var(--accent-2)" }} />
        {status.kind === "busy" ? "Opening Google Drive…" : driveStatusLabel(true)}
      </button>
      <span role="status" aria-live="polite" style={{ fontSize: 11, color: status.kind === "error" ? "var(--error)" : "var(--ink-4)" }}>
        {status.kind === "done" && `Importing “${status.name}”. It will appear above when processed.`}
        {status.kind === "error" && status.message}
        {status.kind === "idle" && "Pick a PDF; SELAR sees only the files you choose."}
      </span>
    </div>
  );
}
