// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { proxy } from "@/proxy";
import { GET as start } from "./start/route";
import { GET as callback } from "./callback/route";
import { GOOGLE_FLOW_COOKIE, decodeFlow, encodeFlow, pkceChallenge, type GoogleFlow } from "@/lib/google-oauth";

const CLIENT_ID = "123-test.apps.googleusercontent.com";
const ORIGIN = "https://console.test";

function cookieValue(res: Response, name: string): string | null {
  for (const c of res.headers.getSetCookie()) {
    if (c.startsWith(`${name}=`)) return decodeURIComponent(c.slice(name.length + 1).split(";")[0]);
  }
  return null;
}

const flow: GoogleFlow = { state: "s".repeat(43), nonce: "n".repeat(43), verifier: "v".repeat(43), from: "/reader" };

function callbackReq(query: string, cookie: string | null = encodeFlow(flow)) {
  const headers = new Headers();
  if (cookie) headers.set("cookie", `${GOOGLE_FLOW_COOKIE}=${cookie}`);
  return new NextRequest(`${ORIGIN}/api/auth/google/callback?${query}`, { headers });
}

beforeEach(() => vi.stubEnv("NEXT_PUBLIC_GOOGLE_CLIENT_ID", CLIENT_ID));
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("google start route", () => {
  it("is 404 when the feature flag is off", async () => {
    vi.stubEnv("NEXT_PUBLIC_GOOGLE_CLIENT_ID", "");
    expect((await start(new NextRequest(`${ORIGIN}/api/auth/google/start`))).status).toBe(404);
  });

  it("redirects to Google with client_id, redirect_uri, PKCE S256, state and nonce", async () => {
    const res = await start(new NextRequest(`${ORIGIN}/api/auth/google/start?from=%2Fgraph`));
    expect(res.status).toBe(307);
    const loc = new URL(res.headers.get("location")!);
    expect(loc.origin + loc.pathname).toBe("https://accounts.google.com/o/oauth2/v2/auth");
    const q = loc.searchParams;
    expect(q.get("client_id")).toBe(CLIENT_ID);
    expect(q.get("redirect_uri")).toBe(`${ORIGIN}/api/auth/google/callback`);
    expect(q.get("response_type")).toBe("code");
    expect(q.get("scope")).toBe("openid email");
    expect(q.get("code_challenge_method")).toBe("S256");

    const stored = decodeFlow(cookieValue(res, GOOGLE_FLOW_COOKIE));
    expect(stored).not.toBeNull();
    expect(q.get("state")).toBe(stored!.state);
    expect(q.get("nonce")).toBe(stored!.nonce);
    expect(q.get("code_challenge")).toBe(await pkceChallenge(stored!.verifier));
    expect(q.get("code_challenge")).not.toBe(stored!.verifier);
    expect(stored!.verifier.length).toBeGreaterThanOrEqual(43);
    expect(stored!.from).toBe("/graph");

    const raw = res.headers.getSetCookie().find((c) => c.startsWith(GOOGLE_FLOW_COOKIE))!;
    expect(raw).toMatch(/HttpOnly/i);
    expect(raw).toMatch(/Path=\/api\/auth\/google/);
    expect(raw).toMatch(/SameSite=lax/i);
  });

  it("uses fresh randomness per attempt and drops unsafe from=", async () => {
    const a = decodeFlow(cookieValue(await start(new NextRequest(`${ORIGIN}/api/auth/google/start?from=//evil.example`)), GOOGLE_FLOW_COOKIE))!;
    const b = decodeFlow(cookieValue(await start(new NextRequest(`${ORIGIN}/api/auth/google/start`)), GOOGLE_FLOW_COOKIE))!;
    expect(a.state).not.toBe(b.state);
    expect(a.verifier).not.toBe(b.verifier);
    expect(a.from).toBe("/library");
  });
});

describe("google callback route", () => {
  it("rejects a missing or mismatched state without calling the API", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    for (const req of [
      callbackReq(`code=c&state=${"x".repeat(43)}`),
      callbackReq(`code=c&state=${flow.state}`, null),
      callbackReq("code=c"),
    ]) {
      const res = await callback(req);
      const loc = new URL(res.headers.get("location")!);
      expect(loc.pathname).toBe("/login");
      expect(loc.searchParams.get("google_error")).toBe("expired");
      expect(cookieValue(res, "selar_token")).toBeNull();
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("reports a cancelled consent screen", async () => {
    const res = await callback(callbackReq(`error=access_denied&state=${flow.state}`));
    expect(new URL(res.headers.get("location")!).searchParams.get("google_error")).toBe("cancelled");
  });

  it("sends code + verifier + nonce to the Go API and sets selar_token on success", async () => {
    const fetchMock = vi.fn(async () => Response.json({ token: "jwt-from-api", user: {} }));
    vi.stubGlobal("fetch", fetchMock);
    const res = await callback(callbackReq(`code=the-code&state=${flow.state}`));
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toMatch(/\/auth\/google$/);
    expect(JSON.parse(String(init.body))).toEqual({
      code: "the-code",
      code_verifier: flow.verifier,
      nonce: flow.nonce,
      redirect_uri: `${ORIGIN}/api/auth/google/callback`,
    });
    expect(new URL(res.headers.get("location")!).pathname).toBe("/reader");
    expect(cookieValue(res, "selar_token")).toBe("jwt-from-api");
    expect(res.headers.getSetCookie().find((c) => c.startsWith("selar_token="))).toMatch(/HttpOnly/i);
    // The one-use flow cookie is cleared.
    expect(cookieValue(res, GOOGLE_FLOW_COOKIE)).toBe("");
  });

  it.each([
    [403, "unverified"],
    [409, "linked_elsewhere"],
    [401, "failed"],
  ])("maps API status %i to google_error=%s", async (status, code) => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ error: "x" }, { status })));
    const res = await callback(callbackReq(`code=c&state=${flow.state}`));
    expect(new URL(res.headers.get("location")!).searchParams.get("google_error")).toBe(code);
    expect(cookieValue(res, "selar_token")).toBeNull();
  });
});

describe("proxy", () => {
  it("lets signed-out visitors reach the Google start and callback routes", () => {
    for (const path of ["/api/auth/google/start", "/api/auth/google/callback?code=c&state=s"]) {
      expect(proxy(new NextRequest(`${ORIGIN}${path}`)).headers.get("location")).toBeNull();
    }
  });
});
