import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SUGGESTIONS_ON_OPEN, suggestionVisibility } from "@/lib/settings";
import { SuggestionVisibilityToggle } from "./SuggestionVisibilityToggle";

const LOCK_MESSAGE = "Set by your study group; it cannot be changed here.";

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); });

async function render(props: { enabled: boolean; locked: boolean; count: number }) {
  await act(async () => root.render(<SuggestionVisibilityToggle {...props} onToggle={vi.fn()} />));
  const button = container.querySelector("button");
  if (!button) throw new Error("toggle button missing");
  return button;
}

/** Text a sighted user sees: everything except visually hidden helpers. */
function visibleText(node: Element): string {
  const clone = node.cloneNode(true) as Element;
  clone.querySelectorAll(".ui-visually-hidden").forEach((el) => el.remove());
  return (clone.textContent ?? "").replace(/\s+/g, " ").trim();
}

describe("SuggestionVisibilityToggle", () => {
  it("disables the Reader toggle and explains a study lock", () => {
    const html = renderToStaticMarkup(
      <SuggestionVisibilityToggle enabled={false} locked count={0} onToggle={vi.fn()} />,
    );

    expect(html).toContain("disabled");
    expect(html).toContain("Set by your study group");
  });

  it("shows a compact locked control instead of running the lock sentence into the count", async () => {
    const button = await render({ enabled: false, locked: true, count: 0 });

    expect(button.disabled).toBe(true);
    // Visible label is short and clearly separated: no "Suggestions 0Set by…" run-on.
    expect(visibleText(container)).toBe("Suggestions: Off 🔒");
    expect(container.textContent).not.toMatch(/\dSet by/);
    // The explanation stays available as a tooltip and to assistive technology.
    expect(button.title).toBe(LOCK_MESSAGE);
    const describedBy = button.getAttribute("aria-describedby");
    expect(describedBy).toBeTruthy();
    expect(document.getElementById(describedBy!)?.textContent).toBe(LOCK_MESSAGE);
    expect(button.getAttribute("aria-label")).toBe("Suggestions: Off (locked by your study group)");
  });

  it("shows the on state and count when the study group locks suggestions on", async () => {
    const button = await render({ enabled: true, locked: true, count: 3 });

    expect(visibleText(container)).toBe("Suggestions: On · 3 🔒");
    expect(button.getAttribute("aria-label")).toBe("Suggestions: On, 3 (locked by your study group)");
  });

  it("keeps the unlocked toggle as a pressable count button", async () => {
    const button = await render({ enabled: true, locked: false, count: 2 });

    expect(button.disabled).toBe(false);
    expect(visibleText(container)).toBe("Suggestions 2");
    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(button.title).toBe("");
    expect(container.textContent).not.toContain("study group");
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
