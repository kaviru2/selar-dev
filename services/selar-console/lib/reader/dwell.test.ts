import { describe, expect, it } from "vitest";
import { createDwellTracker } from "./dwell";

function clock() {
  let t = 0;
  return { now: () => t, advance: (ms: number) => { t += ms; } };
}

describe("createDwellTracker", () => {
  it("counts a page as viewed only after it has been in view for the threshold", () => {
    const c = clock();
    const viewed: number[] = [];
    const tracker = createDwellTracker({ thresholdMs: 1500, now: c.now, onViewed: (page) => viewed.push(page) });
    tracker.setPage(1);
    c.advance(1000);
    tracker.setPage(2); // page 1 had only 1s
    c.advance(200);
    tracker.setPage(3); // flew past page 2
    c.advance(1600);
    tracker.tick();
    expect(viewed).toEqual([3]);
  });

  it("accumulates dwell across visits and reports each page once", () => {
    const c = clock();
    const viewed: number[] = [];
    const tracker = createDwellTracker({ thresholdMs: 1500, now: c.now, onViewed: (page) => viewed.push(page) });
    tracker.setPage(1);
    c.advance(1000);
    tracker.setPage(2);
    c.advance(100);
    tracker.setPage(1);
    c.advance(600);
    tracker.tick();
    c.advance(5000);
    tracker.tick();
    expect(viewed).toEqual([1]);
    expect(tracker.dwell(1)).toBe(6600);
    expect(tracker.dwell(2)).toBe(100);
  });

  it("pauses while the tab is hidden", () => {
    const c = clock();
    const tracker = createDwellTracker({ thresholdMs: 1500, now: c.now, onViewed: () => undefined });
    tracker.setPage(4);
    c.advance(500);
    tracker.pause();
    c.advance(60_000);
    tracker.resume();
    c.advance(500);
    tracker.tick();
    expect(tracker.dwell(4)).toBe(1000);
  });

  it("flush returns and clears per-page dwell not yet reported", () => {
    const c = clock();
    const tracker = createDwellTracker({ thresholdMs: 1500, now: c.now, onViewed: () => undefined });
    tracker.setPage(2);
    c.advance(2500);
    expect(tracker.flush()).toEqual([{ page: 2, ms: 2500 }]);
    c.advance(1000);
    expect(tracker.flush()).toEqual([{ page: 2, ms: 1000 }]);
  });
});
