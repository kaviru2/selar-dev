export interface ReaderSessionSummary {
  pages_viewed: number[];
  max_scroll_depth: number;
}

export interface ReaderTelemetry {
  visitPage(page: number): void;
  recordScrollDepth(depth: number): void;
  showSuggestion(suggestionID: string): void;
  responseTime(suggestionID: string): number;
  sessionSummary(): ReaderSessionSummary;
}

export function createReaderTelemetry(now: () => number = () => performance.now()): ReaderTelemetry {
  const pages = new Set<number>();
  const suggestionShownAt = new Map<string, number>();
  let maxScrollDepth = 0;

  return {
    visitPage(page) {
      if (Number.isInteger(page) && page > 0) pages.add(page);
    },
    recordScrollDepth(depth) {
      if (Number.isFinite(depth)) maxScrollDepth = Math.max(maxScrollDepth, Math.round(Math.max(0, Math.min(100, depth))));
    },
    showSuggestion(suggestionID) {
      if (suggestionID && !suggestionShownAt.has(suggestionID)) {
        suggestionShownAt.set(suggestionID, now());
      }
    },
    responseTime(suggestionID) {
      const shownAt = suggestionShownAt.get(suggestionID);
      if (shownAt === undefined) return 0;
      return Math.max(0, Math.round(now() - shownAt));
    },
    sessionSummary() {
      const pagesViewed = Array.from(pages).sort((left, right) => left - right);
      return {
        pages_viewed: pagesViewed,
        max_scroll_depth: maxScrollDepth,
      };
    },
  };
}
