"use client";

// EmailNoticeSettings — Settings › Reminders: optional study emails
// (issue #114). The API decides whether emails are offered to this account
// and whether a message would really be sent; by default nothing is sent.

import { useEffect, useState } from "react";
import { clientFetch } from "@/lib/api";

export interface NotificationSettingsView {
  available: boolean;
  quiz_emails: boolean;
  security_emails: boolean;
  live: boolean;
}

type Field = "quiz_emails" | "security_emails";

const ROWS: { field: Field; label: string; sub: string }[] = [
  { field: "quiz_emails", label: "Email me when a quiz opens or is about to close", sub: "At most one email per quiz window for each, and never more than a few a day." },
  { field: "security_emails", label: "Email me about account security", sub: "When your password or sign-in email changes." },
];

export function EmailNoticeSettings() {
  const [view, setView] = useState<NotificationSettingsView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    clientFetch<NotificationSettingsView>("/api/users/me/notifications").then(setView).catch(() => setError("Email settings could not be loaded."));
  }, []);

  const set = async (field: Field, value: boolean) => {
    setBusy(true);
    setError(null);
    try {
      setView(await clientFetch<NotificationSettingsView>("/api/users/me/notifications", {
        method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ [field]: value }),
      }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Something went wrong.");
    } finally {
      setBusy(false);
    }
  };

  if (view && !view.available) {
    return (
      <div className="settings-row" data-testid="email-notice-settings">
        <div className="k">
          Study emails
          <span className="sub">Not switched on for this study. The research team decides whether SELAR sends email.</span>
        </div>
        <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>Off for this study</div>
      </div>
    );
  }

  return (
    <div data-testid="email-notice-settings">
      {ROWS.map(({ field, label, sub }) => (
        <div className="settings-row" key={field}>
          <div className="k">
            {label}
            <span className="sub">
              {sub}
              {view && !view.live && " Email delivery is not active yet, so nothing will be sent for now."}
            </span>
          </div>
          <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
            {view ? (
              <div className="toggle" role="group" aria-label={label}>
                <button className={view[field] ? "" : "on"} aria-pressed={!view[field]} disabled={busy} onClick={() => view[field] && set(field, false)}>Off</button>
                <button className={view[field] ? "on" : ""} aria-pressed={view[field]} disabled={busy} onClick={() => !view[field] && set(field, true)}>On</button>
              </div>
            ) : (
              <span>{error ? "—" : "Loading…"}</span>
            )}
          </div>
        </div>
      ))}
      {error && <p role="alert" style={{ color: "#c0443a", fontSize: "var(--t-sm)" }}>{error}</p>}
    </div>
  );
}
