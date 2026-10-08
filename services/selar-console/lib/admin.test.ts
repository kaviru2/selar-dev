import { describe, expect, it } from "vitest";
import { adminQuery, formatDuration, isAdmin, parseAdminFilters, pct, exportHref } from "./admin";

describe("isAdmin", () => {
  it("is true only for role admin", () => {
    expect(isAdmin({ role: "admin" })).toBe(true);
    expect(isAdmin({ role: "user" })).toBe(false);
    expect(isAdmin({})).toBe(false);
    expect(isAdmin(null)).toBe(false);
  });
});

describe("parseAdminFilters", () => {
  it("accepts valid ISO days and a trimmed group", () => {
    expect(parseAdminFilters({ from: "2026-09-01", to: "2026-09-30", group: "  A " })).toEqual({ from: "2026-09-01", to: "2026-09-30", group: "A" });
  });
  it("drops malformed values instead of forwarding them", () => {
    expect(parseAdminFilters({ from: "yesterday", to: ["2026-09-30", "x"], group: "x".repeat(80) })).toEqual({ from: "", to: "2026-09-30", group: "" });
  });
});

describe("adminQuery / exportHref", () => {
  it("only includes set filters", () => {
    expect(adminQuery({ from: "2026-09-01", to: "", group: "" })).toBe("?from=2026-09-01");
    expect(adminQuery({ from: "", to: "", group: "" })).toBe("");
    expect(adminQuery({ from: "", to: "", group: "B & C" })).toBe("?group=B+%26+C");
  });
  it("adds include_email only when explicitly chosen", () => {
    const f = { from: "", to: "", group: "" };
    expect(exportHref("users", f, false)).toBe("/api/admin/export/users.csv");
    expect(exportHref("events", f, true)).toBe("/api/admin/export/events.csv?include_email=true");
  });
});

describe("formatting", () => {
  it("formats durations and rates", () => {
    expect(formatDuration(0)).toBe("0m");
    expect(formatDuration(59_000)).toBe("<1m");
    expect(formatDuration(3_660_000)).toBe("1h 1m");
    expect(pct(1, 4)).toBe("25%");
    expect(pct(0, 0)).toBe("—");
  });
});
