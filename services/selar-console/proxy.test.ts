// @vitest-environment node
import { describe, expect, it } from "vitest";
import { NextRequest } from "next/server";
import { proxy } from "./proxy";

function req(path: string, token?: string) {
  const headers = new Headers();
  if (token !== undefined) headers.set("cookie", `selar_token=${token}`);
  return new NextRequest(new URL(path, "https://console.test"), { headers });
}

function location(res: Response) {
  const loc = res.headers.get("location");
  return loc ? new URL(loc).pathname + new URL(loc).search : null;
}

describe("proxy auth redirects", () => {
  describe("signed-out visitor", () => {
    it("is sent from / to /login (no self-referencing from=)", () => {
      expect(location(proxy(req("/")))).toBe("/login");
    });

    it("sees /login and /register", () => {
      expect(location(proxy(req("/login")))).toBeNull();
      expect(location(proxy(req("/register")))).toBeNull();
    });

    it("does not treat look-alike paths as public", () => {
      expect(location(proxy(req("/loginx")))).toBe("/login?from=%2Floginx");
    });

    it("is sent to /login with from= for protected pages", () => {
      expect(location(proxy(req("/library")))).toBe("/login?from=%2Flibrary");
    });

    it("treats an empty cookie as signed out", () => {
      expect(location(proxy(req("/login", "")))).toBeNull();
    });
  });

  describe("signed-in visitor", () => {
    it("is sent from / into the app", () => {
      expect(location(proxy(req("/", "tok")))).toBe("/library");
    });

    it("is sent from /login and /register into the app", () => {
      expect(location(proxy(req("/login", "tok")))).toBe("/library");
      expect(location(proxy(req("/login/", "tok")))).toBe("/library");
      expect(location(proxy(req("/register", "tok")))).toBe("/library");
    });

    it("honours a safe from= on /login", () => {
      expect(location(proxy(req("/login?from=%2Freader%3Fdoc%3D1", "tok")))).toBe("/reader?doc=1");
    });

    it.each(["//evil.example", "https://evil.example", "/\\evil.example", "/login", "/register"])(
      "ignores unsafe from=%j",
      (from) => {
        const res = proxy(req(`/login?from=${encodeURIComponent(from)}`, "tok"));
        const loc = new URL(res.headers.get("location")!);
        expect(loc.origin).toBe("https://console.test");
        expect(loc.pathname).toBe("/library");
      },
    );

    it("is never redirected away from the session-clearing endpoints", () => {
      expect(location(proxy(req("/api/auth/session-expired", "tok")))).toBeNull();
      expect(location(proxy(req("/api/auth/logout", "tok")))).toBeNull();
    });

    it("reaches protected pages directly", () => {
      expect(location(proxy(req("/library", "tok")))).toBeNull();
    });
  });
});
