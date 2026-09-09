import { describe, expect, it } from "vitest";
import { createReaderTelemetry } from "./reader-telemetry";

describe("reader telemetry", () => {
  it("records unique pages and the highest page reached", () => {
    const telemetry = createReaderTelemetry(() => 1000);

    telemetry.visitPage(2);
    telemetry.visitPage(1);
    telemetry.visitPage(2);
    telemetry.visitPage(5);
    telemetry.recordScrollDepth(23.6);
    telemetry.recordScrollDepth(16);

    expect(telemetry.sessionSummary()).toEqual({
      pages_viewed: [1, 2, 5],
      max_scroll_depth: 24,
    });
  });

  it("measures a suggestion response from its first visible time", () => {
    let now = 1000;
    const telemetry = createReaderTelemetry(() => now);

    telemetry.showSuggestion("link-1");
    now = 2412;

    expect(telemetry.responseTime("link-1")).toBe(1412);
  });

  it("returns zero instead of a fabricated duration for unseen suggestions or backwards clocks", () => {
    let now = 500;
    const telemetry = createReaderTelemetry(() => now);

    expect(telemetry.responseTime("unseen")).toBe(0);
    telemetry.showSuggestion("link-2");
    now = 400;

    expect(telemetry.responseTime("link-2")).toBe(0);
  });
});
