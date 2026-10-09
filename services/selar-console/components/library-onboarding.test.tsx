import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Document } from "@/lib/api";
const mocks = vi.hoisted(() => ({ documents: vi.fn(), stats: vi.fn(), refresh: vi.fn(), drive: vi.fn() }));
vi.mock("@/lib/auth", () => ({ getAuthToken: async () => "synthetic" }));
vi.mock("@/lib/api", () => ({ getDocuments: mocks.documents, getDocumentStats: mocks.stats }));
vi.mock("@/lib/context", () => ({ useSelar: () => ({ user: { id: "test-user" } }) }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: mocks.refresh }) }));
vi.mock("@/components/AddContentButton", () => ({ AddContentButton: () => <button>Add content</button> }));
vi.mock("@/components/SourcesPanel", () => ({ SourcesPanel: () => <div>Sources fixture</div> }));
vi.mock("@/components/DeleteDocButton", () => ({ DeleteDocButton: () => null }));
vi.mock("@/lib/google-drive", () => ({ driveImportEnabled: () => true, driveStatusLabel: () => "Import from Google Drive", importFromDrive: mocks.drive, DriveCancelled: class extends Error {} }));
import LibraryPage from "@/app/(app)/library/page";
import OnboardingPage from "@/app/(app)/onboarding/page";
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
let host: HTMLDivElement; let root: Root;
beforeEach(() => {
  window.localStorage.clear(); vi.clearAllMocks();
  mocks.documents.mockResolvedValue([]);
  mocks.stats.mockResolvedValue({ total_documents: 0, total_chunks: 0, confirmed_links: 0, reading_time_min: 0 });
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
});
afterEach(() => { act(() => root.unmount()); host.remove(); vi.restoreAllMocks(); });
const render = async () => { const page = await LibraryPage(); await act(async () => root.render(page)); };
it("puts all existing import entry points together and hides empty statistics/source management", async () => {
  await render();
  const imports = host.querySelector("#library-imports");
  expect(imports).not.toBeNull();
  expect(imports?.textContent).toContain("Add content");
  expect(imports?.textContent).toContain("Markdown, TXT or DOCX");
  expect(imports?.textContent).toContain("native Google Doc");
  expect(imports?.textContent).toContain("Import from Google Drive");
  expect(host.querySelector('[aria-label="Library summary"]')).toBeNull();
  expect(host.textContent).not.toContain("Sources fixture");
  expect(host.textContent).toContain("Start here");
  mocks.drive.mockResolvedValue("Reading.pdf");
  await act(async () => (imports?.querySelector('[data-testid="drive-footer"] button') as HTMLButtonElement).click());
  expect(mocks.refresh).toHaveBeenCalledOnce();
});
it("does not pretend a library fetch failure is a new user", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  mocks.documents.mockRejectedValue(new Error("offline"));
  await render();
  expect(host.textContent).toContain("couldn't load");
  expect(host.textContent).not.toContain("Start here");
  expect(host.querySelector('[aria-label="Library summary"]')).toBeNull();
});
it("keeps uploaded readings queued and unopenable rather than labeling them failed", async () => {
  mocks.documents.mockResolvedValue([{ id: "queued", title: "Queued notes", status: "uploaded", source_type: "text" } as Document]);
  await render();
  expect(host.textContent).not.toContain("Couldn't process");
  expect(host.querySelector('a[href="/reader?docId=queued"]')).toBeNull();
  expect(host.textContent).toContain("Queued");
});
it("completes the guide when the learner opens a ready reading from the table too", async () => {
  await render();
  mocks.documents.mockResolvedValue([{ id: "ready", title: "Ready notes", status: "ready", source_type: "text" } as Document]);
  await render();
  const link = host.querySelector<HTMLAnchorElement>('.lib-title-link')!;
  link.addEventListener("click", (event) => event.preventDefault());
  await act(async () => link.click());
  expect(window.localStorage.getItem("selar:start-here:v1:test-user")).toBe("complete");
  expect(host.querySelector<HTMLDetailsElement>("#start-here")?.open).toBe(false);
});
it("retains neutral existing-library statistics and keeps source management optional", async () => {
  mocks.documents.mockResolvedValue([{ id: "ready", title: "Ready notes", status: "ready", source_type: "text", source_id: "source" } as Document]);
  await render();
  expect(host.querySelector('[aria-label="Library summary"]')?.textContent).toContain("Historical saved links");
  const sourcePanel = [...host.querySelectorAll("details")].find((d) => d.textContent?.includes("Sources fixture"));
  expect(sourcePanel?.open).toBe(false);
});
it("help leads back to the live guide without a fake completion checklist", async () => {
  await act(async () => root.render(<OnboardingPage />));
  expect(host.querySelector('a[href="/library#start-here"]')).not.toBeNull();
  expect(host.textContent).not.toContain("Create your account");
  expect(host.textContent).not.toMatch(/confirm, reject|formal study/);
});
