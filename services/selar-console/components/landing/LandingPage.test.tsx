import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { LandingPage } from "./LandingPage";

vi.mock("next/link", () => ({ default: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => <a href={href} {...rest}>{children}</a> }));

const html = renderToStaticMarkup(<LandingPage />);
const text = html.replace(/<[^>]+>/g, " ").replace(/&#x27;|&apos;/g, "'").replace(/\s+/g, " ");

describe("public landing page", () => {
  it("offers sign-in and register entry points", () => {
    expect(html).toContain('href="/login"');
    expect(html).toContain('href="/register"');
  });

  it("has the required sections, each labelled for assistive tech", () => {
    for (const id of ["hero-title", "how-title", "research-title", "not-title", "team-title", "cta-title"]) {
      expect(html).toContain(`aria-labelledby="${id}"`);
      expect(html).toContain(`id="${id}"`);
    }
    expect(html.match(/<h1[ >]/g)).toHaveLength(1);
  });

  it("describes optional reflection, source checks and quiet flags without promising saved history", () => {
    for (const step of ["Notice", "Reflect", "Compare", "Continue"]) expect(text).toContain(step);
    expect(text).toContain("not saved");
    expect(text).toContain("not graded");
    expect(text).toContain("This link is wrong");
    expect(text).toContain("does not establish a relationship");
    expect(text).not.toMatch(/keep the link|change it, or reject|reading history|before you see the AI's reason/i);
  });

  it("marks SELAR as a research prototype and credits the team and supervisor", () => {
    expect(text).toContain("research prototype");
    expect(text).toContain("ethics approval");
    for (const name of ["G.P.G.S. Ganegoda", "H.K.S.R. Hapuarachchi", "K.A.D.A.A. Kuruppu Arachchi", "Dr. Thushani A. Weerasinghe"]) {
      expect(text).toContain(name);
    }
  });

  it("only uses the formative survey figures we actually have", () => {
    expect(text).toContain("38 eligible respondents");
    expect(text).toMatch(/31 of 36/);
    expect(text).toMatch(/28 of 36/);
    expect(text).toMatch(/24 of 36/);
  });

  it("cites the learning research with DOI links", () => {
    expect((html.match(/href="https:\/\/doi\.org\//g) || []).length).toBe(4);
    expect(html).toContain('rel="noopener noreferrer"');
  });

  it("makes no outcome claims, testimonials, user counts or pricing", () => {
    const banned = [
      /improves? (your )?(memory|retention|grades|recall)/i,
      /boosts? (your )?(memory|retention|grades)/i,
      /remember (everything|more)/i,
      /proven to/i,
      /guarantee/i,
      /testimonial/i,
      /\btrusted by\b/i,
      /\b\d[\d,]*\+? (users|students|learners)\b/i,
      /\$\d|per month|pricing|\bfree trial\b/i,
    ];
    for (const re of banned) expect(text).not.toMatch(re);
  });
});
