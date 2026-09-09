// page.tsx — Prototype onboarding for initial SELAR testers.

import Link from "next/link";
import { Icon } from "@/components/ui/Icon";

const STEPS = [
  { no: "01", lbl: "Create your account", sub: "Your library is private to your account.", done: true },
  { no: "02", lbl: "Add a document or text", sub: "Start with material you are allowed to use for testing.", done: false },
  { no: "03", lbl: "Read and inspect connections", sub: "Review candidate links before treating them as useful.", done: false },
  { no: "04", lbl: "Share feedback", sub: "Report unclear, incorrect, or useful results to the research team.", done: false },
];

export default function OnboardingPage() {
  return (
    <div className="onboard">
      <div className="left">
        <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 28, fontSize: 15, fontWeight: 600 }}>
          <span className="brand-mark" style={{ width: 18, height: 18, position: "relative", display: "inline-block" }}>
            <span style={{ position: "absolute", inset: 0, background: "var(--ink)", clipPath: "polygon(0 0,100% 0,100% 100%,50% 100%,50% 50%,0 50%)" }} />
            <span style={{ position: "absolute", inset: 0, border: "1px solid var(--ink)", clipPath: "polygon(50% 50%,100% 50%,100% 100%,50% 100%)", background: "var(--accent)" }} />
          </span>
          SELAR
          <span style={{ fontWeight: 400, color: "var(--ink-4)", fontSize: 12, marginLeft: 4 }}>· getting started</span>
        </div>

        <h1>Welcome to SELAR.</h1>
        <div className="lede">
          Add reading material, explore evidence-backed connections, and help us identify what is useful or misleading before any formal study begins.
        </div>

        <div className="steps">
          {STEPS.map((s) => (
            <div key={s.no} className={`step${s.done ? " done" : ""}`}>
              <span className="no">{s.done ? "✓" : s.no}</span>
              <div>
                <div className="lbl">{s.lbl}</div>
                <div className="sub">{s.sub}</div>
              </div>
              <span>
                {s.no === "02" && (
                  <Link href="/library" className="btn primary">
                    Open library <Icon name="arrow_right" size={12} />
                  </Link>
                )}
              </span>
            </div>
          ))}
        </div>

        <div style={{ marginTop: 22, fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)" }}>
          Prototype testing · do not upload sensitive or confidential material
        </div>
      </div>

      <div className="right">
        <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.1em", color: "var(--ink-4)", textTransform: "uppercase" }}>
          What SELAR does
        </div>
        <div style={{ background: "var(--bg)", border: "1px solid var(--rule)", borderRadius: "var(--r-md)", padding: 18, width: 340 }}>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--accent)", textTransform: "uppercase" }}>
            Evidence-backed candidates
          </div>
          <div style={{ fontSize: 18, fontWeight: 600, margin: "4px 0" }}>Review connections yourself</div>
          <div style={{ fontSize: "var(--t-md)", color: "var(--ink-3)", lineHeight: 1.45 }}>
            SELAR surfaces possible connections across your material. Treat suggestions as candidates: inspect their evidence and confirm, reject, or relabel them yourself.
          </div>
        </div>
      </div>
    </div>
  );
}
