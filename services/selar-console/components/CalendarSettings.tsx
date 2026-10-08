"use client";

// CalendarSettings — Settings › Reminders: optional quiz-window calendar
// (issue #113). Everything here is decided by the API: whether the feature
// is switched on for this account, the private feed URL, and the windows,
// which come from the same rules that enforce quiz availability.

import { useEffect, useState } from "react";
import { clientFetch } from "@/lib/api";
import { EmailNoticeSettings } from "@/components/EmailNoticeSettings";

export interface CalendarEventView {
  summary: string;
  start: string;
  end: string;
  google_url: string;
}

export interface CalendarSettingsView {
  available: boolean;
  enabled: boolean;
  feed_url?: string;
  google_subscribe_url?: string;
  events: CalendarEventView[];
}

const sectionLabel: React.CSSProperties = {
  fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase", marginBottom: 6,
};

export function formatWindow(e: CalendarEventView, timeZone?: string): string {
  const fmt = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short", timeZone });
  return `${fmt.format(new Date(e.start))} – ${fmt.format(new Date(e.end))}`;
}

export function CalendarSettings() {
  const [view, setView] = useState<CalendarSettingsView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    clientFetch<CalendarSettingsView>("/api/users/me/calendar").then(setView).catch(() => setError("Calendar settings could not be loaded."));
  }, []);

  const act = async (path: string, init: RequestInit) => {
    setBusy(true);
    setError(null);
    setCopied(false);
    try {
      setView(await clientFetch<CalendarSettingsView>(path, { ...init, headers: { "Content-Type": "application/json" } }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Something went wrong.");
    } finally {
      setBusy(false);
    }
  };

  const copy = async () => {
    if (!view?.feed_url) return;
    try {
      await navigator.clipboard.writeText(view.feed_url);
      setCopied(true);
    } catch {
      setError("Copy failed. Select the link and copy it manually.");
    }
  };

  return (
    <section className="settings-group" aria-label="Reminders" data-testid="calendar-settings">
      <div style={sectionLabel}>Reminders</div>
      <div className="settings-row">
        <div className="k">
          Add my quiz windows to Google Calendar
          <span className="sub">
            {view && !view.available
              ? "Not switched on for this study. The research team decides whether calendar reminders are used."
              : "Optional. A private calendar link that shows when your quizzes open and close. It contains no quiz content."}
          </span>
        </div>
        <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
          {view?.available ? (
            <div className="toggle" role="group" aria-label="Quiz calendar">
              <button className={view.enabled ? "" : "on"} aria-pressed={!view.enabled} disabled={busy}
                onClick={() => view.enabled && act("/api/users/me/calendar", { method: "PUT", body: JSON.stringify({ enabled: false }) })}>
                Off
              </button>
              <button className={view.enabled ? "on" : ""} aria-pressed={view.enabled} disabled={busy}
                onClick={() => !view.enabled && act("/api/users/me/calendar", { method: "PUT", body: JSON.stringify({ enabled: true }) })}>
                On
              </button>
            </div>
          ) : (
            <span>{view ? "Off for this study" : error ? "—" : "Loading…"}</span>
          )}
        </div>
      </div>

      {view?.available && view.enabled && view.feed_url && (
        <>
          <div className="settings-row">
            <div className="k">
              Your private calendar link
              <span className="sub">
                In Google Calendar choose Other calendars › From URL and paste this link, or use the button. Anyone with the link can see your quiz times, so keep it private. Google refreshes subscribed calendars every few hours.
              </span>
            </div>
            <div className="v" style={{ display: "flex", flexDirection: "column", gap: 6, alignItems: "flex-end", maxWidth: 360 }}>
              <input readOnly value={view.feed_url} aria-label="Private calendar link" onFocus={(e) => e.currentTarget.select()}
                style={{ width: "100%", fontFamily: "var(--font-mono)", fontSize: 11 }} />
              <div style={{ display: "flex", gap: 6, flexWrap: "wrap", justifyContent: "flex-end" }}>
                <button className="btn" onClick={copy} disabled={busy}>{copied ? "Copied" : "Copy link"}</button>
                {view.google_subscribe_url && (
                  <a className="btn primary" href={view.google_subscribe_url} target="_blank" rel="noopener noreferrer">Add to Google Calendar</a>
                )}
                <button className="btn" disabled={busy} onClick={() => act("/api/users/me/calendar/reset", { method: "POST" })}>Reset link</button>
              </div>
            </div>
          </div>
          <div className="settings-row">
            <div className="k">
              Upcoming quiz windows
              <span className="sub">Times are shown in your device&apos;s time zone. Single-event links are a snapshot; the calendar link above stays up to date.</span>
            </div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              {view.events.length === 0 ? (
                <span>No upcoming windows.</span>
              ) : (
                <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "grid", gap: 6 }}>
                  {view.events.map((e) => (
                    <li key={`${e.start}-${e.summary}`}>
                      <span>{e.summary.replace(/^SELAR: /, "")} · {formatWindow(e)}</span>{" "}
                      <a href={e.google_url} target="_blank" rel="noopener noreferrer">Add this one</a>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        </>
      )}
      {error && <p role="alert" style={{ color: "#c0443a", fontSize: "var(--t-sm)" }}>{error}</p>}
      <EmailNoticeSettings />
    </section>
  );
}
