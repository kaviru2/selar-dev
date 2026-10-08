// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

const store = { get: vi.fn(), set: vi.fn(), delete: vi.fn() };
vi.mock("next/headers", () => ({ cookies: vi.fn(async () => store) }));

import { getSessionStatus } from "./auth";

afterEach(() => {
  vi.unstubAllGlobals();
  store.get.mockReset();
});

describe("getSessionStatus", () => {
  it("is anonymous without a cookie and does not call the API", async () => {
    store.get.mockReturnValue(undefined);
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    expect(await getSessionStatus()).toEqual({ status: "anonymous" });
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("returns the user for a valid token", async () => {
    store.get.mockReturnValue({ value: "tok" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"id":"u1","email":"a@b.c"}', { status: 200 })));
    expect(await getSessionStatus()).toEqual({ status: "authenticated", user: { id: "u1", email: "a@b.c" } });
  });

  it("reports an invalid token when the API answers 401", async () => {
    store.get.mockReturnValue({ value: "junk" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"error":"unauthorized"}', { status: 401 })));
    expect(await getSessionStatus()).toEqual({ status: "invalid" });
  });

  it("does not treat an API outage as an invalid token (no forced logout)", async () => {
    store.get.mockReturnValue({ value: "tok" });
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("ECONNREFUSED")));
    expect(await getSessionStatus()).toEqual({ status: "unavailable" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 503 })));
    expect(await getSessionStatus()).toEqual({ status: "unavailable" });
  });
});
