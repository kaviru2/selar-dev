// dwell.ts — per-page dwell time for the continuous-scroll reader.
//
// With continuous scroll, a page "passes by" while the learner scrolls. To keep
// `pages_viewed` meaningful for the study, a page is reported as viewed only
// once it has been the current page for `thresholdMs` in total (default 1.5 s),
// and time is not counted while the tab is hidden.

export interface DwellOptions {
  thresholdMs: number;
  now?: () => number;
  onViewed: (page: number, dwellMs: number) => void;
}

export function createDwellTracker({ thresholdMs, now = () => performance.now(), onViewed }: DwellOptions) {
  const total = new Map<number, number>();
  const unflushed = new Map<number, number>();
  const reported = new Set<number>();
  let current = 0;
  let since = now();
  let paused = false;

  function accrue() {
    const at = now();
    if (current > 0 && !paused) {
      const elapsed = Math.max(0, at - since);
      total.set(current, (total.get(current) ?? 0) + elapsed);
      unflushed.set(current, (unflushed.get(current) ?? 0) + elapsed);
      if (!reported.has(current) && (total.get(current) ?? 0) >= thresholdMs) {
        reported.add(current);
        onViewed(current, total.get(current) ?? 0);
      }
    }
    since = at;
  }

  return {
    setPage(page: number) {
      if (page === current) return;
      accrue();
      current = page;
    },
    tick: accrue,
    pause() {
      accrue();
      paused = true;
    },
    resume() {
      since = now();
      paused = false;
    },
    dwell(page: number) {
      return Math.round(total.get(page) ?? 0);
    },
    flush(): Array<{ page: number; ms: number }> {
      accrue();
      const out = Array.from(unflushed.entries())
        .filter(([, ms]) => ms > 0)
        .map(([page, ms]) => ({ page, ms: Math.round(ms) }));
      unflushed.clear();
      return out;
    },
  };
}

export type DwellTracker = ReturnType<typeof createDwellTracker>;
