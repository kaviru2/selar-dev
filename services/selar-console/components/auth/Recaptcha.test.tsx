import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

vi.mock("next/script", () => ({
  default: ({ src, id }: { src: string; id?: string }) => <script data-testid="next-script" id={id} data-src={src} />,
}));

import { getRecaptchaToken, recaptchaEnabled, RecaptchaNotice, RecaptchaScript } from "./Recaptcha";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllEnvs();
  delete window.grecaptcha;
});

describe("reCAPTCHA flag", () => {
  it("is off without NEXT_PUBLIC_RECAPTCHA_SITE_KEY: no script, no notice, no token", async () => {
    vi.stubEnv("NEXT_PUBLIC_RECAPTCHA_SITE_KEY", "");
    expect(recaptchaEnabled()).toBe(false);
    await act(async () => root.render(<><RecaptchaScript /><RecaptchaNotice /></>));
    expect(container.innerHTML).toBe("");
    expect(await getRecaptchaToken("register")).toBeUndefined();
  });

  it("loads enterprise.js with the site key and shows Google's attribution when on", async () => {
    vi.stubEnv("NEXT_PUBLIC_RECAPTCHA_SITE_KEY", "site-key-1");
    expect(recaptchaEnabled()).toBe(true);
    await act(async () => root.render(<><RecaptchaScript /><RecaptchaNotice /></>));
    const script = container.querySelector("[data-testid=next-script]");
    expect(script?.getAttribute("data-src")).toBe("https://www.google.com/recaptcha/enterprise.js?render=site-key-1");
    const notice = container.querySelector("[data-testid=recaptcha-notice]");
    expect(notice?.textContent).toMatch(/protected by reCAPTCHA and the Google Privacy Policy and Terms of Service apply/);
    expect(notice?.querySelector("a[href='https://policies.google.com/privacy']")).toBeTruthy();
    expect(notice?.querySelector("a[href='https://policies.google.com/terms']")).toBeTruthy();
  });

  it("executes the requested action and returns the token", async () => {
    vi.stubEnv("NEXT_PUBLIC_RECAPTCHA_SITE_KEY", "site-key-1");
    const execute = vi.fn().mockResolvedValue("tok-abc");
    window.grecaptcha = { enterprise: { ready: (cb) => cb(), execute } };
    expect(await getRecaptchaToken("register")).toBe("tok-abc");
    expect(execute).toHaveBeenCalledWith("site-key-1", { action: "register" });
  });

  it("returns undefined (API decides) when the script never loads or execute fails", async () => {
    vi.stubEnv("NEXT_PUBLIC_RECAPTCHA_SITE_KEY", "site-key-1");
    expect(await getRecaptchaToken("register", 150)).toBeUndefined();
    window.grecaptcha = { enterprise: { ready: (cb) => cb(), execute: vi.fn().mockRejectedValue(new Error("blocked")) } };
    expect(await getRecaptchaToken("register", 150)).toBeUndefined();
  });
});
