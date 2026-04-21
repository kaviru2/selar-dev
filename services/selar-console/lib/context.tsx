// lib/context.tsx — SELAR app-wide React context provider.
// Provides user state (id, email, cohort), theme, density, and active
// document tracking to all client components via useSelar() hook.

"use client";

import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
} from "react";

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
  const [theme, setThemeState] = useState<Theme>(
    (prefs.theme as Theme) || "paper"
  );
  const [density, setDensityState] = useState<Density>(
    (prefs.density as Density) || "compact"
  );
  const [activeDocId, setActiveDocId] = useState<string | null>(null);

  const setTheme = useCallback(
    async (t: Theme) => {
      setThemeState(t);
      // Apply theme to html element immediately
      document.documentElement.setAttribute(
        "data-theme",
        t === "paper" ? "" : t
      );
      // Persist to API
      try {
        await fetch("/api/auth/preferences", {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ theme: t }),
        });
      } catch {
        // silent fail — theme is already applied locally
      }
    },
    []
  );

  const setDensity = useCallback(
    async (d: Density) => {
      setDensityState(d);
      try {
        await fetch("/api/auth/preferences", {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ density: d }),
        });
      } catch {
        // silent fail
      }
    },
    []
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
