// page.tsx — Settings view.
// Sectioned settings with toggle controls for cohort, theme, density, and privacy.
// Reads directly from global SelarProvider context for real-time reactivity.

"use client";

import { Icon } from "@/components/ui/Icon";
import { ProcessingDisclosure } from "@/components/ProcessingDisclosure";
import { useSelar, type Theme, type Density } from "@/lib/context";
import { useEffect, useState } from "react";
import { clientFetch, type MetricsSummary } from "@/lib/api";

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
  const [metrics, setMetrics] = useState<MetricsSummary | null>(null);

  useEffect(() => {
    clientFetch<MetricsSummary>("/api/evaluation/metrics").then(setMetrics).catch(() => undefined);
  }, []);

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

        {/* Prototype status */}
        <div className="settings-group">
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase", marginBottom: 6 }}>
            Prototype status
          </div>
          <div className="settings-row">
            <div className="k">
              Initial tester access<span className="sub">No study cohort or assessment is active in this build.</span>
            </div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              Evidence-backed link testing
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

        {/* Local evaluation */}
        <div className="settings-group">
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase", marginBottom: 6 }}>
            Local evaluation
          </div>
          <div className="settings-row">
            <div className="k">Grounded chat<span className="sub">Prototype interaction measurements</span></div>
            <div className="v" style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-2)" }}>
              {metrics ? `${metrics.chat_turns} turns · ${Math.round(metrics.average_retrieval_ms)}ms retrieval · ${metrics.average_citations.toFixed(1)} citations/answer` : "No measurements yet"}
            </div>
          </div>
          <div className="settings-row">
            <div className="k">Governance actions<span className="sub">citation use, feedback, and graph review</span></div>
            <div className="v" style={{ fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-2)" }}>
              {metrics ? `${metrics.citation_opens} opens · ${metrics.helpful_answers} helpful · ${metrics.corrections} corrections · ${metrics.confirmed_edges}/${metrics.rejected_edges} confirmed/rejected` : "—"}
            </div>
          </div>
        </div>

        {/* Privacy */}
        <div className="settings-group">
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--ink-4)", textTransform: "uppercase", marginBottom: 6 }}>
             Privacy &amp; data
          </div>
          <ProcessingDisclosure />
          <div className="settings-row">
            <div className="k">
              Export my data
              <span className="sub">Not available in this prototype; do not use it for data you may need to export.</span>
            </div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <button className="btn" disabled>Unavailable</button>
            </div>
          </div>
          <div className="settings-row">
            <div className="k">
              Remove account data
              <span className="sub">Not available in this prototype; contact the research team before adding data you may need removed.</span>
            </div>
            <div className="v" style={{ fontSize: "var(--t-md)", color: "var(--ink-2)" }}>
              <button className="btn" disabled style={{ color: "#c0443a", borderColor: "#c0443a55" }}>
                Unavailable
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
