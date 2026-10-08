// theme.tsx — colour-scheme preference (system / light / dark).
//
// How it works
// - THEME_INIT_SCRIPT runs inline in <head> before first paint and sets
//   <html data-theme="dark"> when needed, so there is no light flash.
// - The preference is stored on the device (localStorage) and, when the user
//   is signed in, in their account preferences as `colorScheme` (see
//   SelarProvider in lib/context.tsx, which registers the remote saver).
// - "system" follows prefers-color-scheme live.
//
// Public API (used by the Settings page and the ThemeToggle):
//   const { preference, resolvedTheme, setPreference } = useTheme();

"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

export type ThemePreference = "system" | "light" | "dark";
export type ResolvedTheme = "light" | "dark";

export const THEME_STORAGE_KEY = "selar-color-scheme";
export const THEME_PREFERENCES: ThemePreference[] = ["system", "light", "dark"];

export function isThemePreference(v: unknown): v is ThemePreference {
  return v === "system" || v === "light" || v === "dark";
}

export function resolveTheme(pref: ThemePreference, systemDark: boolean): ResolvedTheme {
  if (pref === "system") return systemDark ? "dark" : "light";
  return pref;
}

/** Inline, dependency-free; keep in sync with applyTheme(). */
export const THEME_INIT_SCRIPT = `(function(){try{var k=${JSON.stringify(
  THEME_STORAGE_KEY
)};var p=localStorage.getItem(k);if(p!=="light"&&p!=="dark")p="system";var d=p==="dark"||(p==="system"&&window.matchMedia("(prefers-color-scheme: dark)").matches);var e=document.documentElement;if(d){e.setAttribute("data-theme","dark")}else if(e.getAttribute("data-theme")==="dark"){e.removeAttribute("data-theme")}e.style.colorScheme=d?"dark":"light"}catch(_){}})();`;

function applyTheme(theme: ResolvedTheme, animate: boolean) {
  const el = document.documentElement;
  const reduce = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
  if (animate && !reduce) {
    el.classList.add("theme-transition");
    window.setTimeout(() => el.classList.remove("theme-transition"), 260);
  }
  if (theme === "dark") el.setAttribute("data-theme", "dark");
  else el.removeAttribute("data-theme");
  el.style.colorScheme = theme;
}

function readStored(): ThemePreference {
  try {
    const v = window.localStorage.getItem(THEME_STORAGE_KEY);
    return isThemePreference(v) ? v : "system";
  } catch {
    return "system";
  }
}

function systemPrefersDark(): boolean {
  return typeof window !== "undefined" && !!window.matchMedia?.("(prefers-color-scheme: dark)").matches;
}

type RemoteSaver = (pref: ThemePreference) => void | Promise<void>;

interface ThemeState {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setPreference: (pref: ThemePreference) => void;
  /** Internal: lets the signed-in app shell persist the choice to the account. */
  registerRemoteSaver: (saver: RemoteSaver | null) => void;
}

const ThemeContext = createContext<ThemeState | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  // Server render and first client render both use "system"/"light" so markup
  // matches; the inline script has already painted the right colours.
  const [preference, setPref] = useState<ThemePreference>("system");
  const [systemDark, setSystemDark] = useState(false);
  const saver = useRef<RemoteSaver | null>(null);

  useEffect(() => {
    // Sync from storage and the OS after hydration.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPref(readStored());
    setSystemDark(systemPrefersDark());
    const mq = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (!mq) return;
    const onChange = (e: MediaQueryListEvent) => setSystemDark(e.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  const resolvedTheme = resolveTheme(preference, systemDark);
  const mounted = useRef(false);
  useEffect(() => {
    applyTheme(resolvedTheme, mounted.current);
    mounted.current = true;
  }, [resolvedTheme]);

  // Keep several tabs in step.
  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === THEME_STORAGE_KEY) setPref(isThemePreference(e.newValue) ? e.newValue : "system");
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  const setPreference = useCallback((pref: ThemePreference) => {
    setPref(pref);
    try {
      window.localStorage.setItem(THEME_STORAGE_KEY, pref);
    } catch {
      // private mode: the choice still applies for this visit
    }
    void saver.current?.(pref);
  }, []);

  const registerRemoteSaver = useCallback((s: RemoteSaver | null) => {
    saver.current = s;
  }, []);

  const value = useMemo(
    () => ({ preference, resolvedTheme, setPreference, registerRemoteSaver }),
    [preference, resolvedTheme, setPreference, registerRemoteSaver]
  );
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

const FALLBACK: ThemeState = {
  preference: "system",
  resolvedTheme: "light",
  setPreference: () => {},
  registerRemoteSaver: () => {},
};

/** Colour-scheme preference. Safe to call outside the provider (no-op). */
export function useTheme(): Omit<ThemeState, "registerRemoteSaver"> {
  return useContext(ThemeContext) ?? FALLBACK;
}

/** Internal hook for SelarProvider: account-level persistence. */
export function useThemeAccountSync(accountPref: unknown, save: RemoteSaver) {
  const ctx = useContext(ThemeContext) ?? FALLBACK;
  const { registerRemoteSaver, setPreference } = ctx;
  const applied = useRef(false);
  useEffect(() => {
    registerRemoteSaver(save);
    return () => registerRemoteSaver(null);
  }, [registerRemoteSaver, save]);
  useEffect(() => {
    // The account's saved choice wins once per session load, so it follows
    // the user to a new device.
    if (applied.current || !isThemePreference(accountPref)) return;
    applied.current = true;
    if (readStored() !== accountPref) {
      registerRemoteSaver(null);
      setPreference(accountPref);
      registerRemoteSaver(save);
    }
  }, [accountPref, registerRemoteSaver, save, setPreference]);
}
