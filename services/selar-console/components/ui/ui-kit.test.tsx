import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { Button, buttonClass } from "./Button";
import { Badge, PageHeader } from "./Card";
import { EmptyState } from "./EmptyState";
import { Illustration } from "./Illustration";
import { Wordmark } from "./Wordmark";

vi.mock("next/link", () => ({ default: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => <a href={href} {...rest}>{children}</a> }));

describe("shared UI kit", () => {
  it("builds button classes from variant and size", () => {
    expect(buttonClass({ variant: "primary", size: "lg", block: true })).toBe("ui-btn ui-btn--primary ui-btn--lg ui-btn--block");
    expect(buttonClass()).toBe("ui-btn");
  });

  it("renders buttons as type=button by default so they never submit forms by accident", () => {
    expect(renderToStaticMarkup(<Button>Go</Button>)).toContain('type="button"');
    expect(renderToStaticMarkup(<Button type="submit">Go</Button>)).toContain('type="submit"');
  });

  it("hides decorative illustrations from assistive tech, labels titled ones", () => {
    expect(renderToStaticMarkup(<Illustration name="connect" />)).toContain('aria-hidden="true"');
    const titled = renderToStaticMarkup(<Illustration name="hero" title="Two readings linked" />);
    expect(titled).toContain('role="img"');
    expect(titled).toContain('aria-label="Two readings linked"');
  });

  it("renders an empty state with heading, copy and actions", () => {
    const html = renderToStaticMarkup(<EmptyState title="Nothing yet" actions={<Button>Add</Button>}>Add a reading.</EmptyState>);
    expect(html).toContain("<h2>Nothing yet</h2>");
    expect(html).toContain("Add a reading.");
    expect(html).toContain("ui-empty-actions");
  });

  it("renders page header, badge and wordmark", () => {
    expect(renderToStaticMarkup(<PageHeader eyebrow="Library" title="Your readings" />)).toContain("<h1>Your readings</h1>");
    expect(renderToStaticMarkup(<Badge tone="green" dot>Ready</Badge>)).toContain("ui-badge--green");
    expect(renderToStaticMarkup(<Wordmark />)).toContain('aria-label="SELAR home"');
  });
});
