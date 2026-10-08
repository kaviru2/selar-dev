import { afterEach, describe, expect, it, vi } from "vitest";

const setAuthCookie = vi.fn(async (token: string) => void token);
const clearAuthCookie = vi.fn(async () => undefined);
vi.mock("@/lib/auth", () => ({
  getAuthToken: vi.fn(async () => "synthetic-token"),
  setAuthCookie: (t: string) => setAuthCookie(t),
  clearAuthCookie: () => clearAuthCookie(),
}));

import { POST as changePassword } from "./password/route";
import { POST as deleteAccount } from "./delete-account/route";

afterEach(() => {
  vi.unstubAllGlobals();
  setAuthCookie.mockClear();
  clearAuthCookie.mockClear();
});

const post = (body: unknown) =>
  new Request("http://console.test/api/auth/x", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });

describe("change password route", () => {
  it("forwards to the API and swaps in the replacement token without exposing it", async () => {
    const fetchSpy = vi.fn().mockResolvedValue(new Response(JSON.stringify({ token: "new-token", other_sessions_ended: true }), { status: 200 }));
    vi.stubGlobal("fetch", fetchSpy);
    const res = await changePassword(post({ current_password: "a", new_password: "b" }));
    expect(fetchSpy.mock.calls[0][0]).toMatch(/\/api\/users\/me\/password$/);
    expect(fetchSpy.mock.calls[0][1].headers.Authorization).toBe("Bearer synthetic-token");
    expect(setAuthCookie).toHaveBeenCalledWith("new-token");
    const body = await res.json();
    expect(body).toEqual({ other_sessions_ended: true });
  });
  it("passes API errors through and keeps the cookie", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "current password is incorrect" }), { status: 403 })));
    const res = await changePassword(post({ current_password: "x", new_password: "y" }));
    expect(res.status).toBe(403);
    expect(setAuthCookie).not.toHaveBeenCalled();
  });
});

describe("delete account route", () => {
  it("clears the session cookie after a successful deletion", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: "deleted" }), { status: 200 })));
    const res = await deleteAccount(post({ current_password: "a", confirm: "delete my account" }));
    expect(res.status).toBe(200);
    expect(clearAuthCookie).toHaveBeenCalled();
  });
  it("keeps the cookie when the API refuses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "type it" }), { status: 400 })));
    const res = await deleteAccount(post({ current_password: "a", confirm: "nope" }));
    expect(res.status).toBe(400);
    expect(clearAuthCookie).not.toHaveBeenCalled();
  });
});
