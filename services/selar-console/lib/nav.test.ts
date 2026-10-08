import { describe, expect, it } from "vitest";
import { NAV_ITEMS, isActive, visibleNavItems, type NavItem } from "./nav";

describe("app navigation", () => {
  it("lists the shipped sections in order with unique keys", () => {
    expect(NAV_ITEMS.map((i) => i.key)).toEqual(["library", "reader", "chat", "graph", "quizzes", "settings", "admin"]);
    expect(new Set(NAV_ITEMS.map((i) => i.key)).size).toBe(NAV_ITEMS.length);
  });

  it("does not surface study assignment or cohort controls", () => {
    expect(NAV_ITEMS.some((i) => /cohort|study|assignment/i.test(`${i.key} ${i.label} ${i.href}`))).toBe(false);
  });

  it("hides adminOnly items from non-admins and shows them to admins", () => {
    const items: NavItem[] = NAV_ITEMS;
    expect(visibleNavItems(items, undefined).some((i) => i.key === "admin")).toBe(false);
    expect(visibleNavItems(items, "user").some((i) => i.key === "admin")).toBe(false);
    expect(visibleNavItems(items, "admin").some((i) => i.key === "admin")).toBe(true);
  });

  it("matches the active section by path prefix, not substring", () => {
    const reader = NAV_ITEMS.find((i) => i.key === "reader")!;
    expect(isActive(reader, "/reader")).toBe(true);
    expect(isActive(reader, "/reader/abc")).toBe(true);
    expect(isActive(reader, "/readerx")).toBe(false);
    expect(isActive({ ...reader, match: ["/reader", "/history"] }, "/history")).toBe(true);
  });
});
