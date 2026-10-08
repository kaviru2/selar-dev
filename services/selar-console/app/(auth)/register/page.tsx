// page.tsx — Register page.
// Submits to /api/auth/register, which proxies to the Go API, signs the
// new user in, and redirects to /library.

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Icon } from "@/components/ui/Icon";
import { AuthError, AuthField, AuthShell } from "@/components/auth/AuthShell";
import { getRecaptchaToken, RecaptchaNotice, RecaptchaScript } from "@/components/auth/Recaptcha";
import { CONSENT_WORDING } from "@/lib/consent";
import { GoogleSignInButton } from "@/components/auth/GoogleSignInButton";

export default function RegisterPage() {
  const router = useRouter();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  // Optional and unticked by default; the account works the same either way.
  const [researchConsent, setResearchConsent] = useState(false);
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
      const recaptcha_token = await getRecaptchaToken("register");
      const res = await fetch("/api/auth/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password, research_consent: researchConsent, ...(recaptcha_token ? { recaptcha_token } : {}) }),
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "We couldn't create that account. Please try again.");
        return;
      }

      router.push("/library");
      router.refresh();
    } catch {
      setError("We couldn't reach SELAR just now. Check your connection and try again.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthShell
      title="Create your account"
      lede="Add a couple of readings from the same course and see what connections turn up."
      illustration="library"
      aside={{
        heading: "Before you start",
        body: (
          <p>
            SELAR is a research prototype, not a finished product. Suggestions can be wrong, and that&apos;s part of
            the point: you check them against the sources and decide.
          </p>
        ),
      }}
    >
      <AuthError message={error} />
      <GoogleSignInButton />
      <form onSubmit={handleSubmit} className="auth-form">
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
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="Choose a password"
          hint="At least 8 characters."
          required
          minLength={8}
        />
        <AuthField
          id="confirm"
          label="Confirm password"
          type="password"
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          placeholder="Type it once more"
          required
          minLength={8}
        />
        <label className="auth-consent">
          <input type="checkbox" checked={researchConsent} onChange={(e) => setResearchConsent(e.target.checked)} />
          <span>
            {CONSENT_WORDING}. <span className="auth-consent-sub">Optional. You can change this or delete the data later in Settings → Privacy &amp; data.</span>
          </span>
        </label>
        <button type="submit" disabled={loading} className="ui-btn ui-btn--primary ui-btn--lg ui-btn--block">
          {loading ? "Creating account…" : "Create account"}
          {!loading && <Icon name="arrow_right" size={14} />}
        </button>
      </form>
      <RecaptchaScript />
      <RecaptchaNotice />
      <p className="auth-switch">
        Already have an account? <Link href="/login">Sign in</Link>
      </p>
    </AuthShell>
  );
}
