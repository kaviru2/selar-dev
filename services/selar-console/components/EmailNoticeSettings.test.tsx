import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EmailNoticeSettings, type NotificationSettingsView } from "./EmailNoticeSettings";

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.unstubAllGlobals(); });

const ok = (body: NotificationSettingsView) => ({ ok: true, json: async () => body });

describe("EmailNoticeSettings", () => {
  it("shows that email is not switched on for the study, with no toggles", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(ok({ available: false, quiz_emails: false, security_emails: false, live: false })));
    await act(async () => root.render(<EmailNoticeSettings />));
    expect(container.textContent).toMatch(/Not switched on for this study/);
    expect(container.querySelectorAll(".toggle").length).toBe(0);
  });

  it("is off by default, says nothing is sent in dry-run, and opts in per family", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(ok({ available: true, quiz_emails: false, security_emails: false, live: false }))
      .mockResolvedValueOnce(ok({ available: true, quiz_emails: true, security_emails: false, live: false }));
    vi.stubGlobal("fetch", fetchMock);
    await act(async () => root.render(<EmailNoticeSettings />));
    expect(container.textContent).toMatch(/nothing will be sent for now/);
    const pressed = Array.from(container.querySelectorAll('button[aria-pressed="true"]')).map((b) => b.textContent);
    expect(pressed).toEqual(["Off", "Off"]);
    const quizOn = container.querySelectorAll(".toggle")[0].querySelectorAll("button")[1] as HTMLButtonElement;
    await act(async () => quizOn.click());
    expect(fetchMock).toHaveBeenLastCalledWith("/api/users/me/notifications", expect.objectContaining({ method: "PUT", body: JSON.stringify({ quiz_emails: true }) }));
    expect(container.querySelectorAll(".toggle")[0].querySelector('button[aria-pressed="true"]')?.textContent).toBe("On");
  });
});
