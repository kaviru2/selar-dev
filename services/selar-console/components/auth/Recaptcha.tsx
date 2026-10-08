// Recaptcha.tsx — Google reCAPTCHA Enterprise (score-based, invisible) for
// account creation. Rendered only by pages that create accounts (/register,
// and the Google sign-up path), so no other page loads Google's script.
//
// Off unless NEXT_PUBLIC_RECAPTCHA_SITE_KEY is set at build time: local dev
// and tests send no token and the API (without RECAPTCHA_*) does not ask.
// The floating badge is hidden; Google then requires the attribution text,
// which <RecaptchaNotice /> renders under the form.

"use client";

import Script from "next/script";

type Enterprise = {
  ready: (cb: () => void) => void;
  execute: (siteKey: string, opts: { action: string }) => Promise<string>;
};
declare global {
  interface Window {
    grecaptcha?: { enterprise?: Enterprise };
  }
}

export function recaptchaSiteKey(): string {
  return (process.env.NEXT_PUBLIC_RECAPTCHA_SITE_KEY || "").trim();
}

export function recaptchaEnabled(): boolean {
  return recaptchaSiteKey() !== "";
}

/** Loads enterprise.js once; renders nothing when reCAPTCHA is off. */
export function RecaptchaScript() {
  const key = recaptchaSiteKey();
  if (!key) return null;
  return (
    <>
      <Script
        id="recaptcha-enterprise"
        src={`https://www.google.com/recaptcha/enterprise.js?render=${encodeURIComponent(key)}`}
        strategy="afterInteractive"
      />
      {/* Badge hidden; the notice below is Google's required alternative. */}
      <style>{".grecaptcha-badge{visibility:hidden}"}</style>
    </>
  );
}

/** Google's required attribution when the badge is hidden. */
export function RecaptchaNotice() {
  if (!recaptchaEnabled()) return null;
  return (
    <p className="auth-recaptcha" data-testid="recaptcha-notice">
      This site is protected by reCAPTCHA and the Google{" "}
      <a href="https://policies.google.com/privacy" target="_blank" rel="noopener noreferrer">Privacy Policy</a> and{" "}
      <a href="https://policies.google.com/terms" target="_blank" rel="noopener noreferrer">Terms of Service</a> apply.
    </p>
  );
}

/**
 * Returns a token for `action`, or undefined when reCAPTCHA is off or the
 * script never became ready (blocked or offline). The API then decides: it
 * refuses a missing token when verification is enabled.
 */
export async function getRecaptchaToken(action: string, timeoutMs = 8000): Promise<string | undefined> {
  const key = recaptchaSiteKey();
  if (!key || typeof window === "undefined") return undefined;
  const started = Date.now();
  while (!window.grecaptcha?.enterprise) {
    if (Date.now() - started > timeoutMs) return undefined;
    await new Promise((r) => setTimeout(r, 100));
  }
  const enterprise = window.grecaptcha.enterprise;
  try {
    return await new Promise<string>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error("recaptcha timeout")), timeoutMs);
      enterprise.ready(() => {
        enterprise.execute(key, { action }).then(
          (t) => { clearTimeout(timer); resolve(t); },
          (e) => { clearTimeout(timer); reject(e); },
        );
      });
    });
  } catch {
    return undefined;
  }
}
