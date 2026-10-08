import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/auth", () => ({ getAuthToken: vi.fn(async () => "synthetic-token") }));

import { GET } from "./route";

afterEach(() => vi.unstubAllGlobals());

describe("Go API proxy", () => {
  it("passes storage redirects to the browser instead of streaming object bytes", async () => {
    const fetchSpy = vi.fn().mockResolvedValue(new Response(null, {
      status: 302, headers: { Location: "https://bucket.example/doc.pdf?X-Amz-Signature=sig", "Cache-Control": "private, no-store" },
    }));
    vi.stubGlobal("fetch", fetchSpy);
    const response = await GET(new Request("http://console.test/api/documents/d/pdf"), { params: Promise.resolve({ go_api: ["documents", "d", "pdf"] }) });
    expect(fetchSpy.mock.calls[0][1].redirect).toBe("manual");
    expect(fetchSpy.mock.calls[0][1].headers.get("Authorization")).toBe("Bearer synthetic-token");
    expect(response.status).toBe(302);
    expect(response.headers.get("Location")).toBe("https://bucket.example/doc.pdf?X-Amz-Signature=sig");
    expect(response.headers.get("Cache-Control")).toBe("private, no-store");
  });

  it("clears the selar_token cookie when the Go API rejects the token with 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"error":"invalid token"}', { status: 401, headers: { "Content-Type": "application/json" } })));
    const response = await GET(new Request("http://console.test/api/documents"), { params: Promise.resolve({ go_api: ["documents"] }) });
    expect(response.status).toBe(401);
    expect(await response.json()).toEqual({ error: "invalid token" });
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toMatch(/selar_token=;/);
    expect(setCookie).toMatch(/Max-Age=0|Expires=Thu, 01 Jan 1970/i);
  });

  it("does not clear the cookie on 403 or 5xx", async () => {
    for (const status of [403, 500, 503]) {
      vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("{}", { status, headers: { "Content-Type": "application/json" } })));
      const response = await GET(new Request("http://console.test/api/documents"), { params: Promise.resolve({ go_api: ["documents"] }) });
      expect(response.status).toBe(status);
      expect(response.headers.get("set-cookie")).toBeNull();
    }
  });

  it("still streams ordinary JSON responses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"ok":true}', { status: 200, headers: { "Content-Type": "application/json" } })));
    const response = await GET(new Request("http://console.test/api/documents"), { params: Promise.resolve({ go_api: ["documents"] }) });
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ ok: true });
    expect(response.headers.get("Location")).toBeNull();
  });
});
