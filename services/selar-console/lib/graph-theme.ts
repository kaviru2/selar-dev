"use client";

import { useCallback, useEffect, useState } from "react";
import type { GraphNodeType } from "./api";

// The same semantic roles feed DOM legends and canvas drawing. No second palette.
export const REL_COLORS: Record<string, string> = {
  prerequisite_of: "var(--accent-warm-ink)",
  related_to: "var(--ink-3)",
  sub_concept_of: "var(--text-amber)",
  contradicts: "var(--error)",
  extends: "var(--accent)",
  concept_overlap: "var(--text-sky)",
  claim_extension: "var(--accent)",
  assumption_conflict: "var(--error)",
  question_resolution: "var(--text-amber)",
  has_claim: "var(--ink-3)",
  has_assumption: "var(--ink-3)",
  raises: "var(--ink-3)",
  uses_concept: "var(--text-sky)",
};

export const NODE_TYPE_COLORS: Record<GraphNodeType, string> = {
  concept: "var(--text-sky)",
  entity: "var(--accent-2)",
  document: "var(--accent-warm-ink)",
  claim: "var(--accent)",
  assumption: "var(--text-amber)",
  question: "var(--text-amber)",
};

const roles = ["bg", "ink", "ink-3", "accent", "accent-2", "accent-warm-ink", "text-sky", "text-amber", "error", "rule-2"];

/** Canvas cannot resolve var(). Read CSS after hydration and when the theme changes.
 * Updating the resolver repaints the canvas without refetching or rebuilding nodes.
 */
export function useGraphColors() {
  const [colors, setColors] = useState<Record<string, string>>({});
  useEffect(() => {
    const root = document.documentElement;
    const refresh = () => {
      const style = getComputedStyle(root);
      setColors(Object.fromEntries(roles.map(role => [`var(--${role})`, style.getPropertyValue(`--${role}`).trim()])));
    };
    refresh();
    const observer = new MutationObserver(refresh);
    observer.observe(root, { attributes: true, attributeFilter: ["data-theme", "data-paper"] });
    return () => observer.disconnect();
  }, []);
  return useCallback((role: string) => colors[role] || "transparent", [colors]);
}
