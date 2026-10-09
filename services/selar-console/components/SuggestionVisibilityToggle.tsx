import { useId } from "react";
import { Icon } from "@/components/ui/Icon";

interface SuggestionVisibilityToggleProps {
  enabled: boolean;
  locked: boolean;
  count: number;
  onToggle: () => void;
}

export const SUGGESTION_LOCK_MESSAGE = "Set by your study group; it cannot be changed here.";

export function SuggestionVisibilityToggle({ enabled, locked, count, onToggle }: SuggestionVisibilityToggleProps) {
  const lockId = useId();
  if (!locked) {
    return (
      <span className="suggestion-visibility-control">
        <button
          type="button"
          className={enabled ? "on suggestion-toggle" : "suggestion-toggle"}
          aria-pressed={enabled}
          onClick={onToggle}
        >
          <Icon name="link" size={12} /> Suggestions {count}
        </button>
      </span>
    );
  }
  // Locked by the study group: a compact, clearly separated read-only state.
  // The full explanation lives in the tooltip and is announced via
  // aria-describedby, so it never spills into (and wraps) the toolbar.
  const state = enabled ? `On · ${count}` : "Off";
  const spokenState = enabled ? `On, ${count}` : "Off";
  return (
    <span className="suggestion-visibility-control">
      <button
        type="button"
        className={enabled ? "on suggestion-toggle is-locked" : "suggestion-toggle is-locked"}
        disabled
        title={SUGGESTION_LOCK_MESSAGE}
        aria-label={`Suggestions: ${spokenState} (locked by your study group)`}
        aria-describedby={lockId}
        onClick={onToggle}
      >
        <Icon name="link" size={12} /> {`Suggestions: ${state}`} <span aria-hidden="true">🔒</span>
      </button>
      <span id={lockId} className="ui-visually-hidden">{SUGGESTION_LOCK_MESSAGE}</span>
    </span>
  );
}
