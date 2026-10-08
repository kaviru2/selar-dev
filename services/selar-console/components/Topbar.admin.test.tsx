import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

let mockUser: { email: string; role?: string } = { email: "user@example.invalid", role: "user" };
vi.mock("next/link", () => ({ default: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a> }));
vi.mock("next/navigation", () => ({ usePathname: () => "/library", useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }) }));
vi.mock("@/lib/context", () => ({ useSelar: () => ({ user: mockUser }) }));

describe("admin navigation", () => {
  it("is hidden from regular users", async () => {
    mockUser = { email: "user@example.invalid", role: "user" };
    const { Topbar } = await import("./Topbar");
    expect(renderToStaticMarkup(<Topbar />)).not.toContain('href="/admin"');
  });
  it("is shown to admins", async () => {
    mockUser = { email: "admin@example.invalid", role: "admin" };
    const { Topbar } = await import("./Topbar");
    expect(renderToStaticMarkup(<Topbar />)).toContain('href="/admin"');
  });
});
