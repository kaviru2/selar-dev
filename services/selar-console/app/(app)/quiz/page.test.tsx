import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import QuizPage from "./page";

vi.mock("next/navigation", () => ({ notFound: () => { throw new Error("UNFINISHED_ASSESSMENT_DISABLED"); } }));

describe("pre-study assessment gate", () => {
  it("returns not-found rather than rendering a fake delayed test", () => {
    expect(() => renderToStaticMarkup(<QuizPage />)).toThrow("UNFINISHED_ASSESSMENT_DISABLED");
  });
});
