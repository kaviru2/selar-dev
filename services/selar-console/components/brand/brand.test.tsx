import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Linny, LINNY_LABELS, Logo, LogoMark, Wordmark, type LinnyPose } from "./index";

describe("brand components", () => {
  it("renders the logo lockup and mark with an accessible name by default", () => {
    const logo = renderToStaticMarkup(<Logo size={40} />);
    expect(logo).toContain('aria-label="SELAR"');
    expect(logo).toContain("<path");
    expect(renderToStaticMarkup(<LogoMark />)).toContain('role="img"');
  });

  it("hides decorative marks from assistive tech when title is empty", () => {
    const mark = renderToStaticMarkup(<LogoMark title="" />);
    expect(mark).toContain('aria-hidden="true"');
    expect(mark).not.toContain("aria-label");
    expect(renderToStaticMarkup(<Wordmark title="" />)).toContain('aria-hidden="true"');
  });

  it("renders every Linny pose with its own label", () => {
    for (const pose of Object.keys(LINNY_LABELS) as LinnyPose[]) {
      const html = renderToStaticMarkup(<Linny pose={pose} />);
      expect(html).toContain(`data-pose="${pose}"`);
      expect(html).toContain(LINNY_LABELS[pose]);
    }
  });
});
