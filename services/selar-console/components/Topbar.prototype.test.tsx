import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { Topbar } from "./Topbar";

vi.mock("next/link", () => ({ default: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a> }));
vi.mock("next/navigation", () => ({ usePathname: () => "/library", useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }) }));
vi.mock("@/lib/context", () => ({ useSelar: () => ({ user: { email: "prototype@example.invalid", cohort: "treatment_hitl" } }) }));

describe("pre-study prototype navigation", () => {
  it("does not offer the unfinished assessment or display a study assignment", () => {
    const html = renderToStaticMarkup(<Topbar />);
    expect(html).not.toContain('href="/quiz"');
    expect(html).not.toContain("cohort-chip");
    expect(html).not.toContain("HITL");
    expect(html).toContain('href="/reader"');
  });
});
