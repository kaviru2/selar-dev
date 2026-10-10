import { afterEach, describe, expect, it, vi } from "vitest";
import { clearPracticeCache, loadPractice, peekPractice } from "./practice-cache";

afterEach(() => clearPracticeCache());

describe("practice cache", () => {
  it("serves the last value only to the same user", async () => {
    await loadPractice("alice", "daily", async () => ({ items: [1] }));
    expect(peekPractice("alice", "daily")).toEqual({ items: [1] });
    expect(peekPractice("bob", "daily")).toBeUndefined();
  });

  it("never caches without a user id", async () => {
    const fetcher = vi.fn(async () => 1);
    await loadPractice(null, "daily", fetcher);
    await loadPractice(undefined, "daily", fetcher);
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(peekPractice(null, "daily")).toBeUndefined();
  });

  it("shares one in-flight request", async () => {
    let resolve!: (v: number) => void;
    const fetcher = vi.fn(() => new Promise<number>((r) => (resolve = r)));
    const a = loadPractice("alice", "progress", fetcher);
    const b = loadPractice("alice", "progress", fetcher);
    resolve(7);
    expect(await a).toBe(7);
    expect(await b).toBe(7);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("expires entries after five minutes", async () => {
    await loadPractice("alice", "daily", async () => 1);
    expect(peekPractice("alice", "daily", Date.now() + 4 * 60_000)).toBe(1);
    expect(peekPractice("alice", "daily", Date.now() + 6 * 60_000)).toBeUndefined();
  });

  it("clears one user or everyone (logout)", async () => {
    await loadPractice("alice", "daily", async () => 1);
    await loadPractice("bob", "daily", async () => 2);
    clearPracticeCache("alice");
    expect(peekPractice("alice", "daily")).toBeUndefined();
    expect(peekPractice("bob", "daily")).toBe(2);
    clearPracticeCache();
    expect(peekPractice("bob", "daily")).toBeUndefined();
  });

  it("does not cache failures", async () => {
    await expect(loadPractice("alice", "daily", async () => { throw new Error("down"); })).rejects.toThrow("down");
    expect(peekPractice("alice", "daily")).toBeUndefined();
  });
});
