// practice-cache.ts — per-tab stale-while-revalidate cache for the learner's
// own practice reads (daily review queue, progress report).
//
// Going Review -> Reader -> Review used to show "Loading due practice…" every
// time. Now the last answer is shown immediately and refreshed in the
// background. The cache is in-memory only (never localStorage, never a shared
// HTTP cache), keyed by user id, short-lived, and cleared on logout, so one
// account never sees another account's practice data.

const MAX_AGE_MS = 5 * 60 * 1000;

interface Entry<T> {
  value: T;
  at: number;
}

const entries = new Map<string, Entry<unknown>>();
const inflight = new Map<string, Promise<unknown>>();

function keyFor(userId: string | null | undefined, name: string): string | null {
  return userId ? `${userId}:${name}` : null;
}

/** Last cached value for this user, or undefined when absent or expired. */
export function peekPractice<T>(userId: string | null | undefined, name: string, now = Date.now()): T | undefined {
  const key = keyFor(userId, name);
  if (!key) return undefined;
  const hit = entries.get(key);
  if (!hit) return undefined;
  if (now - hit.at > MAX_AGE_MS) {
    entries.delete(key);
    return undefined;
  }
  return hit.value as T;
}

/**
 * Fetch fresh data and remember it. Concurrent calls for the same key share
 * one request. Without a user id nothing is cached.
 */
export function loadPractice<T>(userId: string | null | undefined, name: string, fetcher: () => Promise<T>): Promise<T> {
  const key = keyFor(userId, name);
  if (!key) return fetcher();
  const pending = inflight.get(key);
  if (pending) return pending as Promise<T>;
  const request = fetcher()
    .then((value) => {
      entries.set(key, { value, at: Date.now() });
      return value;
    })
    .finally(() => inflight.delete(key));
  inflight.set(key, request);
  return request;
}

/** Drop cached practice data: after an attempt (one user) or on logout (all). */
export function clearPracticeCache(userId?: string | null): void {
  if (!userId) {
    entries.clear();
    inflight.clear();
    return;
  }
  for (const key of [...entries.keys()]) if (key.startsWith(`${userId}:`)) entries.delete(key);
  for (const key of [...inflight.keys()]) if (key.startsWith(`${userId}:`)) inflight.delete(key);
}
