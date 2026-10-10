import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import DailyReview from "@/app/(app)/review/page";
import Progress from "@/app/(app)/progress/page";
import { SelarProvider } from "@/lib/context";
import { clearPracticeCache } from "@/lib/practice-cache";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }), usePathname: () => "/review" }));

const user = { id: "learner-1", email: "learner@example.invalid", cohort: "control" as const, preferences: {} };
const daily = { status: "ready", items: [{ id: "i1", question: "Recall the method", label: "AI-generated practice" }], streak: 2 };
const progress = { warmup: 1, reading_check: 1, review: 2, exposed: 1, unscored: 0, delayed_unassisted: 1, delayed_scored: 1, delayed_mean_score: 0.5, streak: 2, due: 1 };

function json(body: unknown) {
  return new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json" } });
}

async function mount(node: React.ReactNode) {
  const el = document.createElement("div");
  const root = createRoot(el);
  await act(async () => root.render(<SelarProvider initialUser={user as never}>{node}</SelarProvider>));
  return { el, root };
}

afterEach(() => {
  clearPracticeCache();
  vi.unstubAllGlobals();
});

it("revisiting /review shows the cached queue at once instead of 'Loading due practice…'", async () => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  let release!: () => void;
  const fetcher = vi.fn(async () => json(daily));
  vi.stubGlobal("fetch", fetcher);
  const first = await mount(<DailyReview />);
  expect(first.el.textContent).toContain("Recall the method");
  await act(async () => first.root.unmount());

  // Second visit: the network is slow, but the cached queue renders immediately.
  vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((r) => (release = () => r(json(daily))))));
  const second = await mount(<DailyReview />);
  expect(second.el.textContent).toContain("Recall the method");
  expect(second.el.textContent).not.toContain("Loading due practice");
  await act(async () => release());
  await act(async () => second.root.unmount());
});

it("first /review visit shows an accessible skeleton while loading", async () => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  let release!: () => void;
  vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((r) => (release = () => r(json(daily))))));
  const { el, root } = await mount(<DailyReview />);
  const status = el.querySelector('[role="status"][aria-busy="true"]');
  expect(status?.textContent).toContain("Loading due practice");
  await act(async () => release());
  expect(el.textContent).toContain("Recall the method");
  await act(async () => root.unmount());
});

it("revisiting /progress shows the cached report at once", async () => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  vi.stubGlobal("fetch", vi.fn(async () => json(progress)));
  const first = await mount(<Progress />);
  expect(first.el.textContent).toContain("1 items due");
  await act(async () => first.root.unmount());
  vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(() => undefined)));
  const second = await mount(<Progress />);
  expect(second.el.textContent).toContain("1 items due");
  expect(second.el.textContent).not.toContain("Loading observations");
  await act(async () => second.root.unmount());
});
