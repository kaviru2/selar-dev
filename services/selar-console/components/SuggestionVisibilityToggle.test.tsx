import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { SUGGESTIONS_ON_OPEN, suggestionVisibility } from "@/lib/settings";
import { SuggestionVisibilityToggle } from "./SuggestionVisibilityToggle";

describe("SuggestionVisibilityToggle", () => {
  it("disables the Reader toggle and explains a study lock", () => {
    const html = renderToStaticMarkup(
      <SuggestionVisibilityToggle enabled={false} locked count={0} onToggle={vi.fn()} />,
    );

    expect(html).toContain("disabled");
    expect(html).toContain("Set by your study group");
    expect(html).toContain("Suggestions 0");
  });

  it("uses the effective setting returned by the API", () => {
    expect(suggestionVisibility({
      values: { [SUGGESTIONS_ON_OPEN]: false },
      locked: [SUGGESTIONS_ON_OPEN],
    }, true)).toEqual({ enabled: false, locked: true });
    expect(suggestionVisibility({
      values: { [SUGGESTIONS_ON_OPEN]: true },
      locked: [SUGGESTIONS_ON_OPEN],
    }, false)).toEqual({ enabled: true, locked: true });
  });
});
