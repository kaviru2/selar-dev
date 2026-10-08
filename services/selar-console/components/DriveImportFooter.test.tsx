import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const importFromDrive = vi.fn();
vi.mock("@/lib/google-drive", async (orig) => {
  const real = await orig<typeof import("@/lib/google-drive")>();
  return { ...real, importFromDrive: () => importFromDrive() };
});

import { DriveImportFooter } from "./DriveImportFooter";
import { DriveCancelled } from "@/lib/google-drive";

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  importFromDrive.mockReset();
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllEnvs();
});

const click = (el: Element) => act(async () => { (el as HTMLElement).click(); });

describe("Drive sidebar footer", () => {
  it("never claims Drive is connected", async () => {
    for (const env of [{}, { NEXT_PUBLIC_GOOGLE_CLIENT_ID: "1-x.apps.googleusercontent.com", NEXT_PUBLIC_GOOGLE_PICKER_API_KEY: "k" }]) {
      vi.unstubAllEnvs();
      for (const [k, v] of Object.entries(env)) vi.stubEnv(k, v);
      await act(async () => root.render(<DriveImportFooter />));
      expect(container.textContent).not.toMatch(/connected/i);
    }
  });

  it("says import is not set up, with no button, when the env vars are missing", async () => {
    vi.stubEnv("NEXT_PUBLIC_GOOGLE_CLIENT_ID", "1-x.apps.googleusercontent.com");
    vi.stubEnv("NEXT_PUBLIC_GOOGLE_PICKER_API_KEY", "");
    await act(async () => root.render(<DriveImportFooter />));
    expect(container.textContent).toContain("Google Drive import not set up");
    expect(container.querySelector("button")).toBeNull();
  });

  describe("when configured", () => {
    beforeEach(() => {
      vi.stubEnv("NEXT_PUBLIC_GOOGLE_CLIENT_ID", "1-x.apps.googleusercontent.com");
      vi.stubEnv("NEXT_PUBLIC_GOOGLE_PICKER_API_KEY", "k");
    });

    it("offers Import from Google Drive and reports the import", async () => {
      importFromDrive.mockResolvedValue("Week 3.pdf");
      const onImported = vi.fn();
      await act(async () => root.render(<DriveImportFooter onImported={onImported} />));
      const button = container.querySelector("button")!;
      expect(button.textContent).toContain("Import from Google Drive");
      await click(button);
      expect(container.querySelector("[role=status]")!.textContent).toContain("Importing “Week 3.pdf”");
      expect(onImported).toHaveBeenCalledOnce();
    });

    it("stays quiet when the user cancels and shows API errors", async () => {
      importFromDrive.mockRejectedValueOnce(new DriveCancelled("cancelled"));
      await act(async () => root.render(<DriveImportFooter />));
      await click(container.querySelector("button")!);
      expect(container.querySelector("[role=status]")!.textContent).toMatch(/only the files you choose/);

      importFromDrive.mockRejectedValueOnce(new Error("Only PDF files can be imported from Google Drive"));
      await click(container.querySelector("button")!);
      expect(container.querySelector("[role=status]")!.textContent).toBe("Only PDF files can be imported from Google Drive");
    });
  });
});
