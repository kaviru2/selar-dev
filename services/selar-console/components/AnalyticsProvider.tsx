// components/AnalyticsProvider.tsx — turns first-party analytics on only for
// users who opted in, shows the one-time optional consent prompt, and records
// app sessions and route views (paths only). See docs/ANALYTICS.md.

"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { usePathname } from "next/navigation";
import { analytics, track } from "@/lib/analytics";

export { CONSENT_WORDING } from "@/lib/consent";


interface ConsentUser {
  id: string;
  consented_at?: string | null;
  consent_decided_at?: string | null;
}

interface ConsentState {
  consented: boolean;
  serverEnabled: boolean;
  setConsent: (granted: boolean, via: "prompt" | "settings") => Promise<boolean>;
  /** Apply a consent change saved elsewhere (e.g. the Settings page). */
  sync: (consented: boolean) => void;
}

const ConsentContext = createContext<ConsentState>({ consented: false, serverEnabled: false, setConsent: async () => false, sync: () => undefined });
export const useAnalyticsConsent = () => useContext(ConsentContext);

export function AnalyticsProvider({ user, children }: { user: ConsentUser | null; children: ReactNode }) {
  const pathname = usePathname();
  const [consented, setConsented] = useState(Boolean(user?.consented_at));
  const [decided, setDecided] = useState(Boolean(user?.consent_decided_at || user?.consented_at));
  const [serverEnabled, setServerEnabled] = useState(false);
  const sessionStart = useRef<number | null>(null);

  useEffect(() => {
    if (!user) return;
    let active = true;
    fetch("/api/analytics/config")
      .then((res) => (res.ok ? res.json() : { enabled: false }))
      .then((cfg: { enabled?: boolean }) => { if (active) setServerEnabled(Boolean(cfg.enabled)); })
      .catch(() => undefined);
    return () => { active = false; };
  }, [user]);

  const on = Boolean(user) && serverEnabled && consented;
  useEffect(() => {
    analytics.setEnabled(on);
    if (!on) return;
    sessionStart.current = Date.now();
    track("app_session_started", { route: window.location.pathname });
    const onHide = () => {
      if (document.visibilityState === "hidden") {
        if (sessionStart.current !== null) {
          track("app_session_ended", { duration_ms: Math.min(Date.now() - sessionStart.current, 12 * 3600 * 1000) });
          sessionStart.current = null;
        }
        analytics.flushOnHide();
      } else if (sessionStart.current === null) {
        sessionStart.current = Date.now();
        track("app_session_started", { route: window.location.pathname });
      }
    };
    document.addEventListener("visibilitychange", onHide);
    window.addEventListener("pagehide", onHide);
    return () => {
      document.removeEventListener("visibilitychange", onHide);
      window.removeEventListener("pagehide", onHide);
      analytics.flushOnHide();
    };
  }, [on]);

  useEffect(() => {
    if (on && pathname) track("page_viewed", { route: pathname });
  }, [on, pathname]);

  const setConsent = useCallback(async (granted: boolean, via: "prompt" | "settings") => {
    try {
      const res = await fetch("/api/users/me/research-consent", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ granted, via }),
      });
      if (!res.ok) return false;
      const body = await res.json().catch(() => ({}));
      setConsented(Boolean(body.consented_at ?? granted));
      setDecided(true);
      if (granted && !serverEnabled) setServerEnabled(true);
      analytics.setEnabled(Boolean(body.consented_at ?? granted));
      return true;
    } catch {
      return false;
    }
  }, [serverEnabled]);

  const sync = useCallback((next: boolean) => {
    setConsented(next);
    setDecided(true);
  }, []);

  return (
    <ConsentContext.Provider value={{ consented, serverEnabled, setConsent, sync }}>
      {children}
      {user && serverEnabled && !decided && <ConsentPrompt onAnswer={(granted) => setConsent(granted, "prompt")} />}
    </ConsentContext.Provider>
  );
}

function ConsentPrompt({ onAnswer }: { onAnswer: (granted: boolean) => Promise<boolean> }) {
  const [busy, setBusy] = useState(false);
  const answer = async (granted: boolean) => {
    setBusy(true);
    const ok = await onAnswer(granted);
    if (!ok) setBusy(false);
  };
  return (
    <div role="dialog" aria-labelledby="consent-title" aria-describedby="consent-body" className="consent-prompt">
      <h2 id="consent-title">Help improve SELAR?</h2>
      <p id="consent-body">
        SELAR is a university research prototype. If you agree, we record how you use the app: which pages you open, how
        long you read, and which suggestions you keep, change or reject. <b>Nothing you type is recorded</b>, only its length.
        The data stays in SELAR&apos;s own database, with no third-party trackers. You can switch this off or delete
        your data at any time in Settings → Privacy &amp; data. Saying no changes nothing about how SELAR works for you.
      </p>
      <div className="consent-actions">
        <button type="button" className="ui-btn ui-btn--sm" disabled={busy} onClick={() => answer(true)}>Yes, record my usage</button>
        <button type="button" className="ui-btn ui-btn--sm" disabled={busy} onClick={() => answer(false)}>No thanks</button>
      </div>
    </div>
  );
}
