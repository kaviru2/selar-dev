import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TakeQuiz } from "./TakeQuiz";
import type { AttemptView } from "@/lib/quiz/quiz";

const q = (id: string, index: number, type: "single_choice" | "free_recall") => ({
  id, index, type, prompt: `DEMO prompt ${id}`, points: 1,
  options: type === "single_choice" ? [{ id: "o1", text: "Alpha" }, { id: "o2", text: "Beta" }] : undefined,
});

const baseView = (over: Partial<AttemptView> = {}): AttemptView => ({
  attempt_id: "att-1", quiz_id: "quiz-1", title: "DEMO quiz", description: "", kind: "practice",
  started_at: "2026-10-08T10:00:00Z", server_now: "2026-10-08T10:00:00Z", no_going_back: false,
  total: 2, current_position: 0, questions: [q("q1", 0, "single_choice"), q("q2", 1, "free_recall")], answers: {},
  ...over,
});

let container: HTMLDivElement;
let root: Root;
let fetchSpy: ReturnType<typeof vi.fn>;
let assign: ReturnType<typeof vi.fn>;
let view: AttemptView;

const json = (body: unknown, status = 200) => Promise.resolve(new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }));
const settle = () => act(async () => { for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0)); });
const button = (name: RegExp) => Array.from(container.querySelectorAll("button")).find((b) => name.test(b.textContent || "")) as HTMLButtonElement | undefined;
const click = async (el: HTMLElement | undefined) => { expect(el).toBeTruthy(); await act(async () => { el!.click(); }); await settle(); };
const type = async (field: HTMLTextAreaElement, value: string) => {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(field, value);
    field.dispatchEvent(new Event("input", { bubbles: true }));
  });
};
const calls = (suffix: string) => fetchSpy.mock.calls.filter(([url]) => String(url).endsWith(suffix));
const render = async () => { await act(async () => root.render(<TakeQuiz initial={view} onDone={assign} />)); await settle(); };

beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div"); document.body.append(container); root = createRoot(container);
  view = baseView();
  assign = vi.fn();
  fetchSpy = vi.fn((url: string) => {
    if (url.endsWith("/answers")) return json({ saved_at: "2026-10-08T10:00:05Z" });
    if (url.endsWith("/submit")) return json({ attempt_id: "att-1" });
    if (url.endsWith("/advance")) return json(baseView({ no_going_back: true, current_position: 1, questions: [q("q2", 1, "free_recall")] }));
    return json({});
  });
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => { act(() => root.unmount()); container.remove(); vi.unstubAllGlobals(); vi.useRealTimers(); });

describe("TakeQuiz", () => {
  it("autosaves a choice immediately and shows progress", async () => {
    await render();
    const radio = container.querySelector('input[type="radio"][value="o2"]') as HTMLInputElement;
    await click(radio);
    const saves = calls("/api/quiz-attempts/att-1/answers");
    expect(saves).toHaveLength(1);
    expect(JSON.parse(saves[0][1].body)).toMatchObject({ question_id: "q1", selected: ["o2"], text: "" });
    expect(container.textContent).toMatch(/1 of 2 answered/);
    expect(container.textContent).toMatch(/Saved/);
  });

  it("debounces text autosave", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await render();
    await click(button(/^Next/));
    const ta = container.querySelector("textarea") as HTMLTextAreaElement;
    await type(ta, "DEMO a");
    await type(ta, "DEMO answer");
    expect(calls("/answers")).toHaveLength(0);
    await act(async () => { vi.advanceTimersByTime(1500); });
    await settle();
    const saves = calls("/answers");
    expect(saves).toHaveLength(1);
    expect(JSON.parse(saves[0][1].body).text).toBe("DEMO answer");
  });

  it("asks for confirmation, warns about unanswered questions, then submits", async () => {
    await render();
    await click(button(/Submit/));
    const dialog = container.querySelector('[role="alertdialog"]');
    expect(dialog?.textContent).toMatch(/2 questions are unanswered/);
    expect(calls("/submit")).toHaveLength(0);
    await click(button(/Keep working/));
    expect(container.querySelector('[role="alertdialog"]')).toBeNull();
    await click(button(/Submit/));
    await click(button(/Yes, submit/));
    expect(calls("/api/quiz-attempts/att-1/submit")).toHaveLength(1);
    expect(assign).toHaveBeenCalledWith("att-1");
  });

  it("in no-going-back mode hides Previous and confirms before moving on", async () => {
    view = baseView({ no_going_back: true, questions: [q("q1", 0, "single_choice")] });
    await render();
    expect(button(/Previous/)).toBeUndefined();
    expect(container.textContent).toMatch(/cannot return/i);
    await click(button(/Next/));
    expect(container.querySelector('[role="alertdialog"]')?.textContent).toMatch(/cannot come back/i);
    await click(button(/Yes, continue/));
    expect(calls("/api/quiz-attempts/att-1/advance")).toHaveLength(1);
    expect(container.textContent).toMatch(/DEMO prompt q2/);
    expect(container.textContent).not.toMatch(/DEMO prompt q1/);
  });

  it("finishes when the server says the attempt is already over", async () => {
    fetchSpy.mockImplementation((url: string) => (url.endsWith("/answers") ? json({ error: "attempt already submitted" }, 409) : json({})));
    await render();
    await click(container.querySelector('input[type="radio"][value="o1"]') as HTMLInputElement);
    expect(assign).toHaveBeenCalledWith("att-1");
  });
});
