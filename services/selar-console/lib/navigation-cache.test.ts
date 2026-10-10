// Guards the navigation-speed settings: a short, browser-only router cache
// and functions pinned next to the Neon database (us-east-1 / iad1).
import { readFileSync } from "node:fs";
import path from "node:path";
import { expect, it } from "vitest";
import nextConfig from "../next.config";

it("reuses visited pages briefly in the browser router cache", () => {
  const dynamic = nextConfig.experimental?.staleTimes?.dynamic ?? 0;
  expect(dynamic).toBeGreaterThan(0);
  expect(dynamic).toBeLessThanOrEqual(60);
});

it("pins console and API functions to iad1, next to the database", () => {
  for (const file of ["../vercel.json", "../../selar-api/vercel.json"]) {
    const config = JSON.parse(readFileSync(path.resolve(__dirname, file), "utf8"));
    expect(config.regions).toEqual(["iad1"]);
  }
});
