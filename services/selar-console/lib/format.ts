// format.ts — Small display-string helpers shared by the library and reader.

/** "1 page", "2 pages", "0 pages". */
export function pageCountLabel(count: number): string {
  return `${count} ${count === 1 ? "page" : "pages"}`;
}

/**
 * Label for the Reader's "go to next suggested passage" button. With no
 * suggestions the button is disabled, so it must not read like an action.
 */
export function nextMatchLabel(totalSuggestions: number, onCurrentPage: number): string {
  if (totalSuggestions <= 0) return "No suggestions";
  if (onCurrentPage > 0) return `${onCurrentPage} highlighted · Next ›`;
  return "Next match ›";
}
