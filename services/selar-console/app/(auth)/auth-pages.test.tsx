import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const push = vi.fn();
const refresh = vi.fn();
let search = new URLSearchParams();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, refresh }),
  useSearchParams: () => search,
}));
vi.mock("next/link", () => ({
  default: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => <a href={href} {...rest}>{children}</a>,
}));

import LoginPage from "./login/page";
import RegisterPage from "./register/page";

let container: HTMLDivElement;
let root: Root;
const settle = () => act(async () => { for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0)); });
const $ = <T extends Element>(sel: string) => container.querySelector(sel) as T;

function type(sel: string, value: string) {
  const input = $<HTMLInputElement>(sel);
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  act(() => {
    setter.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
const submit = () => act(async () => { $<HTMLFormElement>("form").requestSubmit(); });

beforeEach(() => {
  push.mockReset();
  refresh.mockReset();
  search = new URLSearchParams();
  window.matchMedia = vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
});

describe("login page", () => {
  it("has labelled fields, a single h1 and links to register and the landing page", async () => {
    await act(async () => root.render(<LoginPage />));
    expect(container.querySelectorAll("h1")).toHaveLength(1);
    expect($("label[for=email]")?.textContent).toBe("Email");
    expect($("label[for=password]")?.textContent).toBe("Password");
    expect($<HTMLInputElement>("#password").autocomplete).toBe("current-password");
    expect($("a[href='/register']")).toBeTruthy();
    expect($("a[href='/about']")).toBeTruthy();
    expect($("[role=radiogroup][aria-label='Colour theme']")).toBeTruthy();
  });

  it("returns to the safe ?from path after signing in", async () => {
    search = new URLSearchParams("from=/graph");
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await act(async () => root.render(<LoginPage />));
    type("#email", "a@b.edu");
    type("#password", "secret123");
    await submit();
    await settle();
    expect(fetchSpy).toHaveBeenCalledWith("/api/auth/login", expect.objectContaining({ method: "POST" }));
    expect(JSON.parse(String(fetchSpy.mock.calls[0][1]?.body))).toEqual({ email: "a@b.edu", password: "secret123" });
    expect(push).toHaveBeenCalledWith("/graph");
  });

  it("ignores an off-site ?from and shows API errors in an alert", async () => {
    search = new URLSearchParams("from=//evil.example");
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ error: "invalid credentials" }), { status: 401 }));
    await act(async () => root.render(<LoginPage />));
    type("#email", "a@b.edu");
    type("#password", "nope");
    await submit();
    await settle();
    expect($("[role=alert]")?.textContent).toContain("invalid credentials");
    expect(push).not.toHaveBeenCalled();
  });
});

describe("register page", () => {
  it("checks the passwords match before calling the API", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    await act(async () => root.render(<RegisterPage />));
    type("#email", "a@b.edu");
    type("#password", "longenough");
    type("#confirm", "different1");
    await submit();
    expect($("[role=alert]")?.textContent).toContain("Passwords don't match");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("registers and goes to the library", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await act(async () => root.render(<RegisterPage />));
    type("#email", "a@b.edu");
    type("#password", "longenough");
    type("#confirm", "longenough");
    await submit();
    await settle();
    expect(push).toHaveBeenCalledWith("/library");
    expect($<HTMLInputElement>("#password").getAttribute("aria-describedby")).toBe("password-hint");
  });

  it("keeps the research-prototype framing honest", async () => {
    await act(async () => root.render(<RegisterPage />));
    const text = container.textContent || "";
    expect(text).toMatch(/research prototype/i);
    expect(text).not.toMatch(/improve(s|d)? (your )?(memory|retention|grades)/i);
  });
});
