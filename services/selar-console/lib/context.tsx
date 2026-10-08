// lib/context.tsx — SELAR app-wide React context provider.
// Provides user state (id, email, cohort), theme, density, and active
// document tracking to all client components via useSelar() hook.
//
// Colour scheme (light/dark/system) lives in lib/theme.tsx (useTheme()).
// `theme`/`setTheme` here are kept for the existing Settings page: "dark"
// maps to the dark colour scheme, and paper/warm/sage pick a light paper tint.

"use client";

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  type ReactNode,
} from "react";
import { useTheme, useThemeAccountSync, type ThemePreference } from "./theme";

export type Theme = "paper" | "warm" | "sage" | "dark";
export type Density = "compact" | "balanced" | "spacious";
export type Cohort = "control" | "treatment_auto" | "treatment_hitl";

interface SelarUser {
  id: string;
  email: string;
  cohort: Cohort;
  drive_connected: boolean;
  preferences: Record<string, unknown>;
}

interface SelarState {
  user: SelarUser | null;
  theme: Theme;
  density: Density;
  activeDocId: string | null;
  setTheme: (t: Theme) => void;
  setDensity: (d: Density) => void;
  setActiveDocId: (id: string | null) => void;
}

const SelarContext = createContext<SelarState | null>(null);

interface ProviderProps {
  children: ReactNode;
  initialUser: SelarUser | null;
}

export function SelarProvider({ children, initialUser }: ProviderProps) {
  const prefs = initialUser?.preferences ?? {};
  // The API replaces the whole preferences object on PATCH, so always send
  // the merged object to avoid wiping other keys.
  const prefsRef = useRef<Record<string, unknown>>(prefs);
  const savePrefs = useCallback(async (patch: Record<string, unknown>) => {
    prefsRef.current = { ...prefsRef.current, ...patch };
    try {
      await fetch("/api/auth/preferences", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(prefsRef.current),
      });
    } catch {
      // silent fail — the change is already applied locally
    }
  }, []);
  const [paper, setPaper] = useState<Exclude<Theme, "dark">>(
    prefs.theme === "warm" || prefs.theme === "sage" ? prefs.theme : "paper"
  );
  const { resolvedTheme, setPreference } = useTheme();
  const theme: Theme = resolvedTheme === "dark" ? "dark" : paper;

  // Account-level colour scheme: older accounts stored theme: "dark".
  const accountScheme =
    prefs.colorScheme ?? (prefs.theme === "dark" ? "dark" : undefined);
  const saveScheme = useCallback(
    (pref: ThemePreference) => savePrefs({ colorScheme: pref }),
    [savePrefs]
  );
  useThemeAccountSync(accountScheme, saveScheme);
  const [density, setDensityState] = useState<Density>(
    (prefs.density as Density) || "compact"
  );
  const [activeDocId, setActiveDocId] = useState<string | null>(null);

  useEffect(() => {
    const el = document.documentElement;
    if (paper === "paper") el.removeAttribute("data-paper");
    else el.setAttribute("data-paper", paper);
    return () => el.removeAttribute("data-paper");
  }, [paper]);

  const setTheme = useCallback(
    async (t: Theme) => {
      if (t === "dark") {
        setPreference("dark");
        return;
      }
      setPaper(t);
      setPreference("light");
      await savePrefs({ theme: t });
    },
    [setPreference, savePrefs]
  );

  const setDensity = useCallback(
    async (d: Density) => {
      setDensityState(d);
      await savePrefs({ density: d });
    },
    [savePrefs]
  );

  return (
    <SelarContext.Provider
      value={{
        user: initialUser,
        theme,
        density,
        activeDocId,
        setTheme,
        setDensity,
        setActiveDocId,
      }}
    >
      {children}
    </SelarContext.Provider>
  );
}

/**
 * Hook to access SELAR app state from client components.
 * Must be used within a <SelarProvider>.
 */
export function useSelar(): SelarState {
  const ctx = useContext(SelarContext);
  if (!ctx) {
    throw new Error("useSelar must be used within <SelarProvider>");
  }
  return ctx;
}
