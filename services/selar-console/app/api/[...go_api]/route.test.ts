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

  it("still streams ordinary JSON responses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"ok":true}', { status: 200, headers: { "Content-Type": "application/json" } })));
    const response = await GET(new Request("http://console.test/api/documents"), { params: Promise.resolve({ go_api: ["documents"] }) });
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ ok: true });
    expect(response.headers.get("Location")).toBeNull();
  });
});
