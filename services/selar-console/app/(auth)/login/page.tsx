// page.tsx — Login page.
// Submits to /api/auth/login, which proxies to the Go API and sets an
// httpOnly cookie, then returns the user to where they were heading.

"use client";

import { useState, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { Icon } from "@/components/ui/Icon";
import { AuthError, AuthField, AuthShell } from "@/components/auth/AuthShell";
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
        setError(data.error || "That didn't work. Check your email and password and try again.");
        return;
      }

      router.push(from);
      router.refresh();
    } catch {
      setError("We couldn't reach SELAR just now. Check your connection and try again.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthShell
      title="Welcome back"
      lede="Sign in to pick up where your reading left off."
      illustration="hero"
      aside={{
        heading: "Your earlier reading is waiting.",
        body: <p>SELAR notices when today&apos;s passage might connect to something you read before. You get to say how, or whether, it does.</p>,
      }}
    >
      <AuthError message={error} />
      <form onSubmit={handleSubmit} className="auth-form" noValidate={false}>
        <AuthField
          id="email"
          label="Email"
          type="email"
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="you@university.edu"
          required
          autoFocus
        />
        <AuthField
          id="password"
          label="Password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="Your password"
          required
        />
        <button type="submit" disabled={loading} className="ui-btn ui-btn--primary ui-btn--lg ui-btn--block">
          {loading ? "Signing in…" : "Sign in"}
          {!loading && <Icon name="arrow_right" size={14} />}
        </button>
      </form>
      <p className="auth-switch">
        New here? <Link href="/register">Create an account</Link>
      </p>
    </AuthShell>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={<div className="auth-loading">Loading…</div>}>
      <LoginForm />
    </Suspense>
  );
}
