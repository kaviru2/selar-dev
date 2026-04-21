// page.tsx — Settings view.
// Sectioned settings with toggle controls for cohort, theme, density, and privacy.
// Reads directly from global SelarProvider context for real-time reactivity.

"use client";

import { Icon } from "@/components/ui/Icon";
import { useSelar, type Theme, type Density } from "@/lib/context";

const THEMES: { id: Theme; label: string }[] = [
  { id: "paper", label: "Paper" },
  { id: "warm", label: "Warm" },
  { id: "sage", label: "Sage" },
  { id: "dark", label: "Dark" },
];

const DENSITIES: { id: Density; label: string }[] = [
  { id: "compact", label: "compact" },
  { id: "balanced", label: "balanced" },
  { id: "spacious", label: "spacious" },
];

export default function SettingsPage() {
  const { user, theme, density, setTheme, setDensity } = useSelar();

  return (
    <div className="settings-wrap">
      <div className="settings-inner">
        <h1>Settings</h1>

        {/* Account */}
        <div className="settings-group">
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase", marginBottom: 6 }}>
            Account
          </div>
          <div className="settings-row">
            <div className="k">
              Signed in as<span className="sub">via JWT · Email auth</span>
            </div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              {user?.email || "Not signed in"}
            </div>
          </div>
          <div className="settings-row">
            <div className="k">
              Google Drive<span className="sub">PDF source</span>
            </div>
            <div className="v" style={{ display: "flex", alignItems: "center", gap: 8, fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <Icon name="drive" size={12} style={{ color: user?.drive_connected ? "var(--accent-2)" : "var(--ink-4)" }} />
              <span>{user?.drive_connected ? "Connected · /SELAR corpus" : "Not connected"}</span>
              <button className="btn" style={{ marginLeft: 8 }}>
                {user?.drive_connected ? "Disconnect" : "Connect"}
              </button>
            </div>
          </div>
        </div>

        {/* Study */}
        <div className="settings-group">
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase", marginBottom: 6 }}>
            Study
          </div>
          <div className="settings-row">
            <div className="k">
              Cohort<span className="sub">assigned at random</span>
            </div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <div className="toggle">
                {/* Cohort cannot be changed by the user in production, mapped visually for debug */}
                <button className={user?.cohort === "treatment_hitl" ? "on" : ""} disabled>HITL</button>
                <button className={user?.cohort === "treatment_auto" ? "on" : ""} disabled>Auto</button>
                <button className={user?.cohort === "control" ? "on" : ""} disabled>Control</button>
              </div>
            </div>
          </div>
          <div className="settings-row">
            <div className="k">
              Study progress<span className="sub">days remaining</span>
            </div>
            <div className="v" style={{ display: "flex", alignItems: "center", gap: 10, fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <div style={{ width: 180, height: 6, background: "var(--bg-3)", borderRadius: 3 }}>
                <div style={{ width: "0%", height: "100%", background: "var(--accent)", borderRadius: 3 }} />
              </div>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 11 }}>day 0 / 14 · waiting for first reading session</span>
            </div>
          </div>
        </div>

        {/* Appearance */}
        <div className="settings-group">
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase", marginBottom: 6 }}>
            Appearance
          </div>
          <div className="settings-row">
            <div className="k">Theme</div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <div className="toggle">
                {THEMES.map((t) => (
                  <button
                    key={t.id}
                    className={theme === t.id ? "on" : ""}
                    onClick={() => setTheme(t.id)}
                  >
                    {t.label}
                  </button>
                ))}
              </div>
            </div>
          </div>
          <div className="settings-row">
            <div className="k">Density</div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <div className="toggle">
                {DENSITIES.map((d) => (
                  <button
                    key={d.id}
                    className={density === d.id ? "on" : ""}
                    onClick={() => setDensity(d.id)}
                  >
                    {d.label}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </div>

        {/* Privacy */}
        <div className="settings-group">
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase", marginBottom: 6 }}>
             Privacy &amp; data
          </div>
          <div className="settings-row">
            <div className="k">
              Export my data
              <span className="sub">JSON of all annotations, confirmations, quiz responses</span>
            </div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <button className="btn">Download .json</button>
            </div>
          </div>
          <div className="settings-row">
            <div className="k">
              Withdraw from study
              <span className="sub">Irreversible · deletes all link and quiz data</span>
            </div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <button className="btn" style={{ color: "#c0443a", borderColor: "#c0443a55" }}>
                Withdraw
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
