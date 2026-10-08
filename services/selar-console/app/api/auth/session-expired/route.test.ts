// @vitest-environment node
import { describe, expect, it } from "vitest";
import { NextRequest } from "next/server";
import { proxy } from "@/proxy";
import { GET } from "./route";

describe("session-expired route (redirect-loop breaker)", () => {
  it("clears selar_token and sends the browser to /login", async () => {
    const res = await GET(new NextRequest("https://console.test/api/auth/session-expired"));
    expect(res.status).toBeGreaterThanOrEqual(300);
    expect(res.status).toBeLessThan(400);
    expect(new URL(res.headers.get("location")!).pathname).toBe("/login");
    const setCookie = res.headers.get("set-cookie") ?? "";
    expect(setCookie).toMatch(/selar_token=;/);
    expect(setCookie).toMatch(/Max-Age=0|Expires=Thu, 01 Jan 1970/i);
    expect(setCookie).toMatch(/Path=\//);
  });

  it("forwards a safe from= to /login but drops unsafe ones", async () => {
    const ok = await GET(new NextRequest("https://console.test/api/auth/session-expired?from=%2Freader"));
    expect(new URL(ok.headers.get("location")!).searchParams.get("from")).toBe("/reader");
    const bad = await GET(new NextRequest("https://console.test/api/auth/session-expired?from=%2F%2Fevil.example"));
    const badLoc = new URL(bad.headers.get("location")!);
    expect(badLoc.origin).toBe("https://console.test");
    expect(badLoc.searchParams.get("from")).toBeNull();
  });

  it("terminates: an invalid cookie cannot bounce between /login and the app forever", async () => {
    // Simulate a browser with a cookie jar following redirects.
    let cookie: string | null = "junk";
    let path = "/login";
    const visited: string[] = [];
    for (let hop = 0; hop < 10; hop++) {
      visited.push(path);
      const headers = new Headers();
      if (cookie) headers.set("cookie", `selar_token=${cookie}`);
      const request = new NextRequest(new URL(path, "https://console.test"), { headers });
      let res: Response = proxy(request);
      if (!res.headers.get("location")) {
        if (path.startsWith("/api/auth/session-expired")) res = await GET(request);
        // (app) layout: the Go API rejects the junk token -> session-expired.
        else if (path === "/library" && cookie) res = Response.redirect("https://console.test/api/auth/session-expired", 307);
      }
      const setCookie = res.headers.get("set-cookie");
      if (setCookie?.startsWith("selar_token=;")) cookie = null;
      const loc = res.headers.get("location");
      if (!loc) break;
      const u = new URL(loc);
      path = u.pathname + u.search;
    }
    expect(visited.at(-1)).toBe("/login");
    expect(cookie).toBeNull();
    expect(visited.length).toBeLessThan(6);
  });
});
