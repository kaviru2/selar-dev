import { Icon } from "@/components/ui/Icon";

interface SuggestionVisibilityToggleProps {
  enabled: boolean;
  locked: boolean;
  count: number;
  onToggle: () => void;
}

export function SuggestionVisibilityToggle({ enabled, locked, count, onToggle }: SuggestionVisibilityToggleProps) {
  const lockMessage = "Set by your study group; it cannot be changed here.";
  return (
    <span className="suggestion-visibility-control">
      <button
        type="button"
        className={enabled ? "on suggestion-toggle" : "suggestion-toggle"}
        disabled={locked}
        title={locked ? lockMessage : undefined}
        onClick={onToggle}
      >
        <Icon name="link" size={12} /> Suggestions {count}
      </button>
      {locked && <span className="reader-setting-lock">{lockMessage}</span>}
    </span>
  );
}
