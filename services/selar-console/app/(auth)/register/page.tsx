// page.tsx — Register page.
// Registration form for SELAR participants. Submits to /api/auth/register
// which proxies to the Go API, auto-logs in, and redirects to /library.

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Icon } from "@/components/ui/Icon";

export default function RegisterPage() {
  const router = useRouter();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    if (password !== confirm) {
      setError("Passwords don't match");
      return;
    }

    if (password.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }

    setLoading(true);

    try {
      const res = await fetch("/api/auth/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "Registration failed");
        return;
      }

      router.push("/library");
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
      </div>

      <h1 style={{ fontSize: 24, fontWeight: 600, letterSpacing: "-0.02em", margin: "0 0 4px" }}>
        Create your account
      </h1>
      <p style={{ color: "var(--ink-3)", fontSize: "var(--t-md)", marginBottom: 24 }}>
        Join the SELAR study. Your cohort will be assigned automatically.
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
            display: "block", fontSize: "var(--t-sm)", fontWeight: 500, marginBottom: 4, color: "var(--ink-2)",
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
              width: "100%", boxSizing: "border-box", padding: "8px 12px",
              border: "1px solid var(--rule)", borderRadius: "var(--r-sm)",
              fontSize: "var(--t-md)", fontFamily: "inherit",
              background: "var(--bg)", color: "var(--ink)", outline: "none",
            }}
          />
        </div>

        <div>
          <label style={{
            display: "block", fontSize: "var(--t-sm)", fontWeight: 500, marginBottom: 4, color: "var(--ink-2)",
          }}>
            Password
          </label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="At least 8 characters"
            required
            minLength={8}
            style={{
              width: "100%", boxSizing: "border-box", padding: "8px 12px",
              border: "1px solid var(--rule)", borderRadius: "var(--r-sm)",
              fontSize: "var(--t-md)", fontFamily: "inherit",
              background: "var(--bg)", color: "var(--ink)", outline: "none",
            }}
          />
        </div>

        <div>
          <label style={{
            display: "block", fontSize: "var(--t-sm)", fontWeight: 500, marginBottom: 4, color: "var(--ink-2)",
          }}>
            Confirm password
          </label>
          <input
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            placeholder="Repeat password"
            required
            minLength={8}
            style={{
              width: "100%", boxSizing: "border-box", padding: "8px 12px",
              border: "1px solid var(--rule)", borderRadius: "var(--r-sm)",
              fontSize: "var(--t-md)", fontFamily: "inherit",
              background: "var(--bg)", color: "var(--ink)", outline: "none",
            }}
          />
        </div>

        <button
          type="submit"
          disabled={loading}
          className="btn primary"
          style={{ width: "100%", justifyContent: "center", padding: "10px 16px", marginTop: 4 }}
        >
          {loading ? "Creating account…" : "Create account"}
          {!loading && <Icon name="arrow_right" size={13} />}
        </button>
      </form>

      <p style={{ textAlign: "center", marginTop: 20, fontSize: "var(--t-sm)", color: "var(--ink-3)" }}>
        Already have an account?{" "}
        <Link href="/login" style={{ color: "var(--accent)", textDecoration: "none", fontWeight: 500 }}>
          Sign in
        </Link>
      </p>

      <div style={{
        marginTop: 32, paddingTop: 16, borderTop: "1px solid var(--rule)",
        textAlign: "center", fontFamily: "var(--font-mono)", fontSize: 10, color: "var(--ink-4)",
      }}>
        By registering you consent to participate in UCSC Ethics Protocol SELAR-2026-04
      </div>
    </div>
  );
}
