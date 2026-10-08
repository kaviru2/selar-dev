// ThemeToggle.tsx — small light/dark/system switch.
// A radio group (arrow keys move between options) built on useTheme().

"use client";

import { useRef, type KeyboardEvent } from "react";
import { Icon, type IconName } from "@/components/ui/Icon";
import { THEME_PREFERENCES, useTheme, type ThemePreference } from "@/lib/theme";

const META: Record<ThemePreference, { label: string; icon: IconName }> = {
  system: { label: "Match device", icon: "monitor" },
  light: { label: "Light", icon: "sun" },
  dark: { label: "Dark", icon: "moon" },
};

export function ThemeToggle({ className = "" }: { className?: string }) {
  const { preference, setPreference } = useTheme();
  const refs = useRef<Array<HTMLButtonElement | null>>([]);

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = THEME_PREFERENCES.indexOf(preference);
    let next = -1;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = (i + 1) % THEME_PREFERENCES.length;
    if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = (i + THEME_PREFERENCES.length - 1) % THEME_PREFERENCES.length;
    if (next < 0) return;
    e.preventDefault();
    setPreference(THEME_PREFERENCES[next]);
    refs.current[next]?.focus();
  };

  return (
    <div role="radiogroup" aria-label="Colour theme" className={`ui-theme-toggle ${className}`.trim()} onKeyDown={onKeyDown}>
      {THEME_PREFERENCES.map((pref, i) => {
        const on = preference === pref;
        return (
          <button
            key={pref}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="radio"
            aria-checked={on}
            aria-label={META[pref].label}
            title={META[pref].label}
            tabIndex={on ? 0 : -1}
            className={on ? "on" : ""}
            onClick={() => setPreference(pref)}
          >
            <Icon name={META[pref].icon} size={15} />
          </button>
        );
      })}
    </div>
  );
}
