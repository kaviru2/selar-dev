// page.tsx — Public landing page for unauthenticated users.

import Link from "next/link";

const nav: React.CSSProperties = {
  display: "flex", alignItems: "center", padding: "14px 48px", gap: 12,
  position: "sticky", top: 0, zIndex: 10,
  background: "rgba(250,249,247,0.92)", backdropFilter: "blur(16px)",
  WebkitBackdropFilter: "blur(16px)", borderBottom: "1px solid var(--rule)",
};

const heroWrap: React.CSSProperties = {
  maxWidth: 640, margin: "0 auto", padding: "100px 40px 48px", textAlign: "center",
};

const featureGrid: React.CSSProperties = {
  display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16,
  maxWidth: 860, margin: "0 auto", padding: "0 40px 72px",
};

const stepGrid: React.CSSProperties = {
  display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 0,
  maxWidth: 860, margin: "0 auto", padding: "0 40px 80px",
};

const fCard: React.CSSProperties = {
  padding: "24px 20px", border: "1px solid var(--rule)", borderRadius: 6,
  background: "var(--bg)", transition: "box-shadow 0.2s",
};

export default function LandingPage() {
  return (
    <div style={{ minHeight: "100vh", background: "var(--bg)", color: "var(--ink)", fontFamily: "var(--font-sans)", display: "flex", flexDirection: "column" }}>

      {/* ── Nav ── */}
      <nav style={nav}>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span style={{ width: 18, height: 18, position: "relative", display: "inline-block" }}>
            <span style={{ position: "absolute", inset: 0, background: "var(--ink)", clipPath: "polygon(0 0,100% 0,100% 100%,50% 100%,50% 50%,0 50%)" }} />
            <span style={{ position: "absolute", inset: 0, border: "1px solid var(--ink)", clipPath: "polygon(50% 50%,100% 50%,100% 100%,50% 100%)", background: "var(--accent)" }} />
          </span>
          <span style={{ fontSize: 15, fontWeight: 600, letterSpacing: "-0.01em" }}>SELAR</span>
        </div>
        <div style={{ flex: 1 }} />
        <Link href="/login" style={{ fontSize: 13, color: "var(--ink-3)", textDecoration: "none", padding: "6px 14px", borderRadius: 4 }}>Sign in</Link>
        <Link href="/register" style={{ fontSize: 13, fontWeight: 500, color: "#fff", background: "var(--ink)", textDecoration: "none", padding: "7px 18px", borderRadius: 4 }}>Get Started</Link>
      </nav>

      {/* ── Hero ── */}
      <section style={heroWrap}>
        <div style={{
          display: "inline-flex", alignItems: "center", gap: 7,
          fontFamily: "var(--font-mono)", fontSize: 10, letterSpacing: "0.08em", textTransform: "uppercase" as const,
          color: "var(--accent)", border: "1px solid rgba(201,100,66,0.2)", background: "rgba(201,100,66,0.05)",
          borderRadius: 20, padding: "5px 16px", marginBottom: 28,
        }}>
          <span style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--accent)" }} />
          Semantic PDF Reader
        </div>

        <h1 style={{ fontSize: 48, fontWeight: 700, letterSpacing: "-0.04em", lineHeight: 1.08, margin: "0 0 22px" }}>
          Read smarter.
          <br />
          <span style={{ color: "var(--accent)" }}>Remember everything.</span>
        </h1>

        <p style={{
          fontFamily: "var(--font-serif)", fontSize: 17, lineHeight: 1.7, color: "var(--ink-3)",
          maxWidth: 480, margin: "0 auto 36px",
        }}>
          SELAR surfaces hidden connections across your research papers using AI‑powered semantic analysis — turning passive reading into active retention.
        </p>

        <div style={{ display: "flex", justifyContent: "center", gap: 10 }}>
          <Link href="/register" style={{
            fontSize: 14, fontWeight: 500, color: "#fff", background: "var(--ink)",
            textDecoration: "none", padding: "11px 28px", borderRadius: 5,
            border: "1px solid var(--ink)", display: "inline-flex", alignItems: "center", gap: 8,
          }}>
            Start reading
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>
          </Link>
          <Link href="/login" style={{
            fontSize: 14, color: "var(--ink-2)", background: "var(--bg)",
            textDecoration: "none", padding: "11px 28px", borderRadius: 5,
            border: "1px solid var(--rule-2)",
          }}>
            Sign in
          </Link>
        </div>
      </section>

      {/* ── Divider ── */}
      <div style={{ maxWidth: 860, margin: "0 auto", padding: "0 40px", width: "100%", boxSizing: "border-box" }}>
        <div style={{ borderTop: "1px solid var(--rule)", marginBottom: 48 }} />
      </div>

      {/* ── Features ── */}
      <section style={featureGrid}>
        {[
          {
            icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1 0-5H20"/></svg>,
            bg: "rgba(201,100,66,0.07)", color: "var(--accent)",
            title: "Continuous PDF Reader",
            desc: "Scroll through papers naturally with highlights, notes, and page annotations.",
          },
          {
            icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>,
            bg: "rgba(122,140,92,0.07)", color: "var(--accent-2)",
            title: "AI Semantic Links",
            desc: "Automatic cross-document connections via vector embeddings, surfaced as match cards.",
          },
          {
            icon: <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"/><circle cx="19" cy="5" r="2"/><circle cx="5" cy="19" r="2"/><line x1="14.5" y1="9.5" x2="17.5" y2="6.5"/><line x1="9.5" y1="14.5" x2="6.5" y2="17.5"/></svg>,
            bg: "rgba(138,106,61,0.07)", color: "var(--accent-3)",
            title: "Knowledge Graph",
            desc: "Confirmed links build a personal graph connecting concepts across your entire library.",
          },
        ].map((f, i) => (
          <div key={i} style={fCard}>
            <div style={{ width: 36, height: 36, borderRadius: 5, background: f.bg, color: f.color, display: "grid", placeItems: "center", marginBottom: 14 }}>
              {f.icon}
            </div>
            <h3 style={{ fontSize: 14, fontWeight: 600, margin: "0 0 5px", letterSpacing: "-0.01em" }}>{f.title}</h3>
            <p style={{ fontSize: 12, color: "var(--ink-3)", lineHeight: 1.55, margin: 0 }}>{f.desc}</p>
          </div>
        ))}
      </section>

      {/* ── Steps ── */}
      <section style={{ maxWidth: 860, margin: "0 auto 0", padding: "0 40px 80px", width: "100%", boxSizing: "border-box" }}>
        <h2 style={{ fontSize: 20, fontWeight: 600, letterSpacing: "-0.02em", margin: "0 0 24px", textAlign: "center" as const }}>
          How it works
        </h2>
        <div style={stepGrid}>
          {[
            { n: "01", t: "Upload PDFs", d: "Drop papers into your library. SELAR parses, chunks, and embeds every page." },
            { n: "02", t: "Read naturally", d: "Scroll a clean reader. Highlight passages and add margin notes as you go." },
            { n: "03", t: "Review matches", d: "Confirm or reject AI suggestions — each action is retrieval practice." },
            { n: "04", t: "Build your graph", d: "Links weave into a knowledge graph that grows with every paper." },
          ].map((s, i) => (
            <div key={i} style={{ padding: "18px 16px", borderTop: "2px solid var(--rule)" }}>
              <span style={{ fontFamily: "var(--font-mono)", fontSize: 11, fontWeight: 600, color: "var(--accent)", display: "block", marginBottom: 8 }}>{s.n}</span>
              <h4 style={{ fontSize: 13, fontWeight: 600, margin: "0 0 5px" }}>{s.t}</h4>
              <p style={{ fontSize: 11.5, color: "var(--ink-3)", lineHeight: 1.5, margin: 0 }}>{s.d}</p>
            </div>
          ))}
        </div>
      </section>

      {/* ── Footer ── */}
      <footer style={{
        marginTop: "auto", padding: "14px 48px", borderTop: "1px solid var(--rule)",
        display: "flex", alignItems: "center", gap: 10,
        fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)", letterSpacing: "0.02em",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 6, fontWeight: 600, fontSize: 11, color: "var(--ink-3)" }}>
          <span style={{ width: 12, height: 12, position: "relative", display: "inline-block" }}>
            <span style={{ position: "absolute", inset: 0, background: "var(--ink-4)", clipPath: "polygon(0 0,100% 0,100% 100%,50% 100%,50% 50%,0 50%)" }} />
            <span style={{ position: "absolute", inset: 0, border: "1px solid var(--ink-4)", clipPath: "polygon(50% 50%,100% 50%,100% 100%,50% 100%)", background: "var(--accent)" }} />
          </span>
          SELAR
        </div>
        <span>Semantic Linking for Active Retention</span>
        <span style={{ marginLeft: "auto" }}>Built for research. Designed to remember.</span>
      </footer>
    </div>
  );
}
