// page.tsx — Login page.
// Clean, design-system-aligned login form. Submits to /api/auth/login
// which proxies to the Go API and sets an httpOnly cookie.

"use client";

import { useState, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { Icon } from "@/components/ui/Icon";
import { safeRedirectPath } from "@/lib/safe-redirect";

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const from = safeRedirectPath(searchParams.get("from"));

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "Login failed");
        return;
      }

      router.push(from);
      router.refresh();
    } catch {
      setError("Network error — is the API running?");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ width: "100%", maxWidth: 400, padding: "0 24px" }}>
      {/* Brand */}
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 32 }}>
        <span className="brand-mark" style={{ width: 20, height: 20, position: "relative", display: "inline-block" }}>
          <span style={{ position: "absolute", inset: 0, background: "var(--ink)", clipPath: "polygon(0 0,100% 0,100% 100%,50% 100%,50% 50%,0 50%)" }} />
          <span style={{ position: "absolute", inset: 0, border: "1px solid var(--ink)", clipPath: "polygon(50% 50%,100% 50%,100% 100%,50% 100%)", background: "var(--accent)" }} />
        </span>
        <span style={{ fontSize: 18, fontWeight: 600, letterSpacing: "-0.01em" }}>SELAR</span>
        <span style={{ fontSize: 12, color: "var(--ink-4)", fontWeight: 400 }}>Semantic Linking for Active Retention</span>
      </div>

      <h1 style={{ fontSize: 24, fontWeight: 600, letterSpacing: "-0.02em", margin: "0 0 4px" }}>
        Sign in
      </h1>
      <p style={{ color: "var(--ink-3)", fontSize: "var(--t-md)", marginBottom: 24 }}>
        Enter your credentials to access your library.
      </p>

      {error && (
        <div style={{
          padding: "8px 12px",
          borderRadius: "var(--r-sm)",
          border: "1px solid rgba(192,68,58,0.3)",
          background: "rgba(192,68,58,0.06)",
          color: "#c0443a",
          fontSize: "var(--t-sm)",
          marginBottom: 16,
          display: "flex",
          alignItems: "center",
          gap: 6,
        }}>
          <Icon name="x" size={12} /> {error}
        </div>
      )}

      <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <div>
          <label style={{
            display: "block",
            fontSize: "var(--t-sm)",
            fontWeight: 500,
            marginBottom: 4,
            color: "var(--ink-2)",
          }}>
            Email
          </label>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@university.edu"
            required
            autoFocus
            style={{
              width: "100%",
              boxSizing: "border-box",
              padding: "8px 12px",
              border: "1px solid var(--rule)",
              borderRadius: "var(--r-sm)",
              fontSize: "var(--t-md)",
              fontFamily: "inherit",
              background: "var(--bg)",
              color: "var(--ink)",
              outline: "none",
            }}
          />
        </div>

        <div>
          <label style={{
            display: "block",
            fontSize: "var(--t-sm)",
            fontWeight: 500,
            marginBottom: 4,
            color: "var(--ink-2)",
          }}>
            Password
          </label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="••••••••"
            required
            style={{
              width: "100%",
              boxSizing: "border-box",
              padding: "8px 12px",
              border: "1px solid var(--rule)",
              borderRadius: "var(--r-sm)",
              fontSize: "var(--t-md)",
              fontFamily: "inherit",
              background: "var(--bg)",
              color: "var(--ink)",
              outline: "none",
            }}
          />
        </div>

        <button
          type="submit"
          disabled={loading}
          className="btn primary"
          style={{ width: "100%", justifyContent: "center", padding: "10px 16px", marginTop: 4 }}
        >
          {loading ? "Signing in…" : "Sign in"}
          {!loading && <Icon name="arrow_right" size={13} />}
        </button>
      </form>

      <p style={{ textAlign: "center", marginTop: 20, fontSize: "var(--t-sm)", color: "var(--ink-3)" }}>
        Don&apos;t have an account?{" "}
        <Link href="/register" style={{ color: "var(--accent)", textDecoration: "none", fontWeight: 500 }}>
          Register
        </Link>
      </p>

      <div style={{
        marginTop: 32,
        paddingTop: 16,
        borderTop: "1px solid var(--rule)",
        textAlign: "center",
        fontFamily: "var(--font-mono)",
        fontSize: 10,
        color: "var(--ink-4)",
      }}>
        Prototype access · use non-sensitive material for testing
      </div>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={<div style={{ textAlign: "center", width: "100%", padding: 24 }}>Loading...</div>}>
      <LoginForm />
    </Suspense>
  );
}
