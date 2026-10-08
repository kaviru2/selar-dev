import { describe, expect, it } from "vitest";
import { DEFAULT_SIGNED_IN_PATH, safeRedirectPath } from "./safe-redirect";

describe("safeRedirectPath", () => {
  it("defaults to the library when no destination is given", () => {
    expect(DEFAULT_SIGNED_IN_PATH).toBe("/library");
    expect(safeRedirectPath(null)).toBe("/library");
    expect(safeRedirectPath(undefined)).toBe("/library");
    expect(safeRedirectPath("")).toBe("/library");
  });

  it("keeps same-origin relative app paths, including query and hash", () => {
    expect(safeRedirectPath("/reader")).toBe("/reader");
    expect(safeRedirectPath("/reader?doc=abc&page=3")).toBe("/reader?doc=abc&page=3");
    expect(safeRedirectPath("/graph#node-1")).toBe("/graph#node-1");
  });

  it.each([
    "//evil.example",
    "//evil.example/library",
    "/\\evil.example",
    "\\\\evil.example",
    "/library\\..\\x",
    "https://evil.example",
    "javascript:alert(1)",
    "library",
    " /library",
    "/%2F%2Fevil.example",
    "/\t/evil.example",
  ])("rejects unsafe destination %j", (from) => {
    expect(safeRedirectPath(from)).toBe("/library");
  });

  it.each(["/login", "/login?from=/x", "/register", "/register/", "/", "/api/auth/logout", "/api/auth/session-expired"])(
    "rejects auth-loop destination %j",
    (from) => {
      expect(safeRedirectPath(from)).toBe("/library");
    },
  );
});
