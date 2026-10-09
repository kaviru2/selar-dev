// page.tsx — Settings view.
// Every control here is persisted by the API (lib/settings.ts documents the
// keys). Sections: Account, Password, Appearance, Reader, Suggestions,
// Privacy & data. Controls a study can pin per cohort show as "set by your
// study" and cannot be changed.

"use client";

import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { ProcessingDisclosure } from "@/components/ProcessingDisclosure";
import { CalendarSettings } from "@/components/CalendarSettings";
import { useAnalyticsConsent } from "@/components/AnalyticsProvider";
import { ThemeToggle } from "@/components/ui/ThemeToggle";
import { Button } from "@/components/ui/Button";
import { useSelar, type Theme } from "@/lib/context";
import { clientFetch, type MetricsSummary } from "@/lib/api";
import {
  DELETE_ACCOUNT_CONFIRMATION,
  HIGHLIGHT_COLORS,
  SUGGESTIONS_ON_OPEN,
  ZOOM_CHOICES,
  changeEmail,
  changePassword,
  deleteAccount,
  loadSettings,
  passwordProblem,
  readerSettings,
  saveDisplayName,
  saveSettings,
  setResearchConsent,
  suggestionsOnOpen,
  zoomLabel,
  type ReaderSettings,
  type ZoomMode,
} from "@/lib/settings";

const PAPERS: { id: Exclude<Theme, "dark">; label: string }[] = [
  { id: "paper", label: "Paper" },
  { id: "warm", label: "Warm" },
  { id: "sage", label: "Sage" },
];

const label = { fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase" as const, marginBottom: 6 };
const value = { fontSize: "var(--t-md)", color: "var(--ink-2)" };
const input = { font: "inherit", fontSize: "var(--t-md)", padding: "5px 8px", border: "1px solid var(--rule)", borderRadius: 6, background: "var(--bg)", color: "var(--ink)", width: "100%", maxWidth: 360 };
const check = { accentColor: "var(--accent)" };
const SWATCH: Record<string, string> = { yellow: "rgb(255, 220, 79)", green: "rgb(120, 190, 110)", blue: "rgb(110, 160, 230)", pink: "rgb(235, 130, 170)", purple: "rgb(165, 125, 220)" };
const form = { display: "flex", flexDirection: "column" as const, gap: 8, alignItems: "flex-start" };

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="settings-group" aria-label={title}>
      <div style={label}>{title}</div>
      {children}
    </section>
  );
}

function Row({ k, sub, children }: { k: string; sub?: ReactNode; children: ReactNode }) {
  return (
    <div className="settings-row">
      <div className="k">
        {k}
        {sub && <span className="sub">{sub}</span>}
      </div>
      <div className="v" style={value}>{children}</div>
    </div>
  );
}

function Status({ msg }: { msg: { ok: boolean; text: string } | null }) {
  if (!msg) return null;
  return (
    <span role={msg.ok ? "status" : "alert"} style={{ fontSize: "var(--t-sm)", color: msg.ok ? "var(--accent-2)" : "#c0443a" }}>
      {msg.text}
    </span>
  );
}

type Msg = { ok: boolean; text: string } | null;
const fail = (e: unknown): Msg => ({ ok: false, text: e instanceof Error ? e.message : "Something went wrong" });

export default function SettingsPage() {
  const { user, theme, setTheme, preferences, applyPreferences } = useSelar();
  const [locked, setLocked] = useState<string[]>([]);
  const [metrics, setMetrics] = useState<MetricsSummary | null>(null);

  // Profile
  const [displayName, setDisplayName] = useState(user?.display_name ?? "");
  const [profileMsg, setProfileMsg] = useState<Msg>(null);
  const [email, setEmail] = useState(user?.email ?? "");
  const [shownEmail, setShownEmail] = useState(user?.email ?? "");
  const [emailPassword, setEmailPassword] = useState("");
  const [emailMsg, setEmailMsg] = useState<Msg>(null);

  // Password
  const [currentPw, setCurrentPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [pwMsg, setPwMsg] = useState<Msg>(null);

  // Preferences
  // Save feedback is shown in the section that was changed.
  const [prefMsg, setPrefMsg] = useState<{ section: "reader" | "suggestions"; msg: Msg } | null>(null);
  const reader = readerSettings(preferences);
  const showOnOpen = suggestionsOnOpen(preferences);

  // Privacy
  const analyticsConsent = useAnalyticsConsent();
  const [analyticsMsg, setAnalyticsMsg] = useState<Msg>(null);
  const [consentedAt, setConsentedAt] = useState<string | null>(user?.consented_at ?? null);
  const [consentMsg, setConsentMsg] = useState<Msg>(null);
  const [deletePw, setDeletePw] = useState("");
  const [deleteText, setDeleteText] = useState("");
  const [deleteMsg, setDeleteMsg] = useState<Msg>(null);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    clientFetch<MetricsSummary>("/api/evaluation/metrics").then(setMetrics).catch(() => undefined);
    loadSettings()
      .then((s) => {
        setLocked(s.locked);
        applyPreferences(s.values);
      })
      .catch(() => undefined);
  }, [applyPreferences]);

  async function savePref(section: "reader" | "suggestions", patch: Record<string, unknown>) {
    setPrefMsg(null);
    try {
      const s = await saveSettings(patch);
      setLocked(s.locked);
      applyPreferences(s.values);
      setPrefMsg({ section, msg: { ok: true, text: "Saved" } });
    } catch (e) {
      setPrefMsg({ section, msg: fail(e) });
    }
  }
  const saveReader = (patch: Partial<ReaderSettings>) => savePref("reader", { reader: patch });

  async function onProfile(e: FormEvent) {
    e.preventDefault();
    setBusy("profile");
    try {
      const r = await saveDisplayName(displayName);
      setDisplayName(r.display_name);
      setProfileMsg({ ok: true, text: "Saved" });
    } catch (err) {
      setProfileMsg(fail(err));
    } finally {
      setBusy(null);
    }
  }

  async function onEmail(e: FormEvent) {
    e.preventDefault();
    setBusy("email");
    try {
      const r = await changeEmail(email.trim(), emailPassword);
      setShownEmail(r.email);
      setEmailPassword("");
      setEmailMsg({ ok: true, text: "Email updated. Use it the next time you sign in." });
    } catch (err) {
      setEmailMsg(fail(err));
    } finally {
      setBusy(null);
    }
  }

  const pwProblem = newPw ? passwordProblem(newPw, shownEmail) : null;
  async function onPassword(e: FormEvent) {
    e.preventDefault();
    if (pwProblem) return setPwMsg({ ok: false, text: pwProblem });
    if (newPw !== confirmPw) return setPwMsg({ ok: false, text: "The new passwords don't match." });
    setBusy("password");
    try {
      await changePassword(currentPw, newPw);
      setCurrentPw("");
      setNewPw("");
      setConfirmPw("");
      setPwMsg({ ok: true, text: "Password changed. You were signed out everywhere else." });
    } catch (err) {
      setPwMsg(fail(err));
    } finally {
      setBusy(null);
    }
  }

  async function onConsent(granted: boolean) {
    setBusy("consent");
    try {
      const u = await setResearchConsent(granted);
      setConsentedAt(u.consented_at);
      // Keep the shared analytics client in step (no reload needed).
      analyticsConsent.sync(Boolean(u.consented_at));
      setConsentMsg({ ok: true, text: granted ? "Thank you. Usage analytics are on." : "Usage analytics are off." });
    } catch (err) {
      setConsentMsg(fail(err));
    } finally {
      setBusy(null);
    }
  }

  async function onDelete(e: FormEvent) {
    e.preventDefault();
    setBusy("delete");
    try {
      await deleteAccount(deletePw);
      window.location.assign("/");
    } catch (err) {
      setDeleteMsg(fail(err));
      setBusy(null);
    }
  }

  const suggestionsLocked = locked.includes(SUGGESTIONS_ON_OPEN);

  return (
    <div className="settings-wrap">
      <div className="settings-inner">
        <h1>Settings</h1>

        <Group title="Account">
          <Row k="Display name" sub="Shown in the app. Optional.">
            <form onSubmit={onProfile} style={{ ...form, flexDirection: "row", alignItems: "center", flexWrap: "wrap" }}>
              <input aria-label="Display name" style={input} maxLength={80} value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
              <Button size="sm" type="submit" disabled={busy === "profile"}>Save</Button>
              <Status msg={profileMsg} />
            </form>
          </Row>
          <Row k="Email" sub={<>Signed in as {shownEmail || "—"}</>}>
            <form onSubmit={onEmail} style={form}>
              <input aria-label="New email" type="email" required style={input} value={email} onChange={(e) => setEmail(e.target.value)} />
              <input aria-label="Current password (to change email)" type="password" required autoComplete="current-password" placeholder="Current password" style={input} value={emailPassword} onChange={(e) => setEmailPassword(e.target.value)} />
              <Button size="sm" type="submit" disabled={busy === "email" || !emailPassword || email.trim() === shownEmail}>Change email</Button>
              <Status msg={emailMsg} />
            </form>
          </Row>
        </Group>

        <Group title="Password">
          <Row k="Change password" sub="At least 10 characters, mixing letters with numbers, spaces or symbols. Signs you out on every other device.">
            <form onSubmit={onPassword} style={form}>
              <input aria-label="Current password" type="password" required autoComplete="current-password" placeholder="Current password" style={input} value={currentPw} onChange={(e) => setCurrentPw(e.target.value)} />
              <input aria-label="New password" type="password" required autoComplete="new-password" placeholder="New password" style={input} value={newPw} onChange={(e) => setNewPw(e.target.value)} aria-invalid={Boolean(pwProblem)} />
              <input aria-label="Confirm new password" type="password" required autoComplete="new-password" placeholder="Confirm new password" style={input} value={confirmPw} onChange={(e) => setConfirmPw(e.target.value)} />
              {pwProblem && <span style={{ fontSize: "var(--t-sm)", color: "var(--ink-3)" }}>{pwProblem}</span>}
              <Button size="sm" type="submit" disabled={busy === "password" || !currentPw || !newPw}>Change password</Button>
              <Status msg={pwMsg} />
            </form>
          </Row>
        </Group>

        <Group title="Appearance">
          <Row k="Colour theme" sub="Match device, light or dark">
            <ThemeToggle />
          </Row>
          <Row k="Paper tint" sub="Light theme only">
            <div className="toggle" role="radiogroup" aria-label="Paper tint">
              {PAPERS.map((p) => (
                <button key={p.id} type="button" role="radio" aria-checked={theme === p.id} className={theme === p.id ? "on" : ""} onClick={() => setTheme(p.id)}>
                  {p.label}
                </button>
              ))}
            </div>
          </Row>
        </Group>

        <Group title="Reader">
          <Row k="Default zoom" sub="When a document opens">
            <div className="toggle" role="radiogroup" aria-label="Default zoom">
              {ZOOM_CHOICES.map((z: ZoomMode) => (
                <button key={String(z)} type="button" role="radio" aria-checked={reader.defaultZoom === z} className={reader.defaultZoom === z ? "on" : ""} onClick={() => saveReader({ defaultZoom: z })}>
                  {zoomLabel(z)}
                </button>
              ))}
            </div>
          </Row>
          <Row k="Highlight colour" sub="For new highlights">
            <div className="toggle" role="radiogroup" aria-label="Highlight colour">
              {HIGHLIGHT_COLORS.map((c) => (
                <button key={c} type="button" role="radio" aria-checked={reader.highlightColor === c} className={reader.highlightColor === c ? "on" : ""} onClick={() => saveReader({ highlightColor: c })}>
                  <span aria-hidden style={{ display: "inline-block", width: 9, height: 9, borderRadius: 2, marginRight: 5, background: SWATCH[c], verticalAlign: "middle" }} />
                  {c}
                </button>
              ))}
            </div>
          </Row>
          <Row k="Remember position" sub="Reopen documents where you left off (stored on this device)">
            <label style={{ display: "flex", gap: 6, alignItems: "center" }}>
              <input type="checkbox" aria-label="Remember position" style={check} checked={reader.rememberPosition} onChange={(e) => saveReader({ rememberPosition: e.target.checked })} />
              {reader.rememberPosition ? "On" : "Off"}
            </label>
          </Row>
          <Row k="Page thumbnails" sub="Show the thumbnail strip by default">
            <label style={{ display: "flex", gap: 6, alignItems: "center" }}>
              <input type="checkbox" aria-label="Page thumbnails" style={check} checked={reader.showThumbnails} onChange={(e) => saveReader({ showThumbnails: e.target.checked })} />
              {reader.showThumbnails ? "On" : "Off"}
            </label>
          </Row>
          {prefMsg?.section === "reader" && (
            <Row k="">
              <Status msg={prefMsg.msg} />
            </Row>
          )}
        </Group>

        <Group title="Suggestions">
          <Row
            k="Show suggestions when a document opens"
            sub={suggestionsLocked ? "Set by your study group; it cannot be changed here." : "You can still toggle them per document in the Reader."}
          >
            <label style={{ display: "flex", gap: 6, alignItems: "center" }}>
              <input
                type="checkbox"
                aria-label="Show suggestions when a document opens"
                style={check}
                checked={showOnOpen}
                disabled={suggestionsLocked}
                title={suggestionsLocked ? "Set by your study group; it cannot be changed here." : undefined}
                onChange={(e) => savePref("suggestions", { [SUGGESTIONS_ON_OPEN]: e.target.checked })}
              />
              {showOnOpen ? "On" : "Off"}
              {suggestionsLocked && <span aria-hidden>🔒</span>}
            </label>
          </Row>
          {prefMsg?.section === "suggestions" && (
            <Row k="">
              <Status msg={prefMsg.msg} />
            </Row>
          )}
        </Group>

        <CalendarSettings />

        <Group title="Local evaluation">
          <Row k="Grounded chat" sub="Prototype interaction measurements">
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 11 }}>
              {metrics?.chat_turns ? `${metrics.chat_turns} turns · ${Math.round(metrics.average_retrieval_ms ?? 0)}ms retrieval · ${(metrics.average_citations ?? 0).toFixed(1)} citations/answer` : "No measurements yet"}
            </span>
          </Row>
          <Row k="Governance actions" sub="citation use, feedback, and graph review">
            <span style={{ fontFamily: "var(--font-mono)", fontSize: 11 }}>
              {metrics && metrics.citation_opens !== undefined ? `${metrics.citation_opens} opens · ${metrics.helpful_answers} helpful · ${metrics.corrections} corrections · ${metrics.confirmed_edges}/${metrics.rejected_edges} confirmed/rejected` : "—"}
            </span>
          </Row>
        </Group>

        <Group title="Privacy & data">
          <ProcessingDisclosure />
          <Row
            k="Usage analytics"
            sub="Optional. Lets the team see how SELAR is used. This is not the research-study consent form; that is handled separately by the research team."
          >
            <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <span>{consentedAt ? `On since ${new Date(consentedAt).toLocaleDateString()}` : "Off"}</span>
              <Button size="sm" disabled={busy === "consent"} onClick={() => onConsent(!consentedAt)}>
                {consentedAt ? "Turn off" : "Turn on"}
              </Button>
              <Status msg={consentMsg} />
            </div>
          </Row>
          <Row k="Delete my usage analytics" sub="Removes every usage event recorded for your account. Your account and documents stay.">
            <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
              <Button
                size="sm"
                disabled={busy === "analytics"}
                onClick={async () => {
                  if (!window.confirm("Delete all usage analytics recorded for your account? This cannot be undone.")) return;
                  setBusy("analytics");
                  try {
                    const res = await fetch("/api/users/me/analytics", { method: "DELETE" });
                    const body = await res.json().catch(() => ({}));
                    if (!res.ok) throw new Error(body.error || "Could not delete");
                    setAnalyticsMsg({ ok: true, text: `Deleted ${body.deleted_events ?? 0} recorded events.` });
                  } catch (err) {
                    setAnalyticsMsg(fail(err));
                  } finally {
                    setBusy(null);
                  }
                }}
              >
                Delete analytics data
              </Button>
              <Status msg={analyticsMsg} />
            </div>
          </Row>
          <Row k="Export my data" sub="A zip of JSON files: your documents' details, highlights, link decisions, quiz attempts and activity. Uploaded PDFs are not included.">
            <a className="ui-btn ui-btn--sm" href="/api/users/me/export" download>
              Download export
            </a>
          </Row>
          <Row k="Delete my account" sub="Permanently deletes your account, documents, files and all activity. An anonymous record that a participant withdrew (study group and dates only) is kept. This cannot be undone.">
            <form onSubmit={onDelete} style={form}>
              <input aria-label="Current password (to delete account)" type="password" required autoComplete="current-password" placeholder="Current password" style={input} value={deletePw} onChange={(e) => setDeletePw(e.target.value)} />
              <input aria-label={`Type ${DELETE_ACCOUNT_CONFIRMATION} to confirm`} placeholder={`Type “${DELETE_ACCOUNT_CONFIRMATION}”`} style={input} value={deleteText} onChange={(e) => setDeleteText(e.target.value)} />
              <Button
                size="sm"
                variant="warm"
                type="submit"
                disabled={busy === "delete" || !deletePw || deleteText.trim().toLowerCase() !== DELETE_ACCOUNT_CONFIRMATION}
              >
                Delete account
              </Button>
              <Status msg={deleteMsg} />
            </form>
          </Row>
        </Group>
      </div>
    </div>
  );
}
