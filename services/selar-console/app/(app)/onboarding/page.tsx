// page.tsx — Onboarding view.
// Two-column split: steps checklist on the left, cohort assignment card on the right.
// Matches design_handoff_selar §7 — Onboarding.

import { Icon } from "@/components/ui/Icon";

const STEPS = [
  { no: "01", lbl: "Sign in with Google", sub: "OAuth · basic profile only", done: true },
  { no: "02", lbl: "Connect Google Drive", sub: "PDFs stay in your own Drive — SELAR reads, never uploads", done: true },
  { no: "03", lbl: "Receive seed corpus (8 papers)", sub: "Pre-loaded ML fundamentals reading list · ~240 pages", done: true },
  { no: "04", lbl: "Take the 20-minute pre-test", sub: "Establishes baseline retention before the study period", done: false, cta: "Start pre-test" },
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
          <span style={{ fontWeight: 400, color: "var(--ink-4)", fontSize: 12, marginLeft: 4 }}>· onboarding</span>
        </div>

        <h1>Welcome, Ashen.</h1>
        <div className="lede">
          You&apos;re participating in a 14-day study on whether confirming semantic
          links between documents strengthens retention. Before you start reading,
          we need to get a baseline.
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
                {s.cta && (
                  <button className="btn primary">
                    {s.cta} <Icon name="arrow_right" size={12} />
                  </button>
                )}
              </span>
            </div>
          ))}
        </div>

        <div style={{ marginTop: 22, fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)" }}>
          UCSC ethics protocol SELAR-2026-04 · consent signed 18 Apr 2026
        </div>
      </div>

      <div className="right">
        <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.1em", color: "var(--ink-4)", textTransform: "uppercase" }}>
          Your cohort assignment
        </div>
        <div style={{ background: "var(--bg)", border: "1px solid var(--rule)", borderRadius: "var(--r-md)", padding: 18, width: 340 }}>
          <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", color: "var(--accent)", textTransform: "uppercase" }}>
            Treatment · HITL
          </div>
          <div style={{ fontSize: 18, fontWeight: 600, margin: "4px 0" }}>Confirm-then-link</div>
          <div style={{ fontSize: "var(--t-md)", color: "var(--ink-3)", lineHeight: 1.45 }}>
            As you read, SELAR will surface candidate semantic matches in a side panel.
            You decide which to confirm, reject, or relabel. The confirmation itself is
            the study intervention.
          </div>
          <div style={{ borderTop: "1px solid var(--rule)", marginTop: 14, paddingTop: 10, display: "flex", gap: 18, fontSize: 12, color: "var(--ink-3)" }}>
            <span><b style={{ color: "var(--ink)" }}>~10 min</b> reading / day</span>
            <span><b style={{ color: "var(--ink)" }}>14 days</b> active</span>
            <span><b style={{ color: "var(--ink)" }}>3</b> quizzes</span>
          </div>
        </div>
        <div style={{ fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)", maxWidth: 340, textAlign: "center" }}>
          Cohort was assigned at random. You&apos;ll learn what the other cohorts saw
          at debrief on day 22.
        </div>
      </div>
    </div>
  );
}
