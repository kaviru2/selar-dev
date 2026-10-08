import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QuizEditor } from "./QuizEditor";
import { ImportPanel } from "./ImportPanel";

let container: HTMLDivElement;
let root: Root;
let fetchSpy: ReturnType<typeof vi.fn>;
let saved: ReturnType<typeof vi.fn>;

const json = (body: unknown, status = 200) => Promise.resolve(new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }));
const settle = () => act(async () => { for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0)); });
const button = (name: RegExp) => Array.from(container.querySelectorAll("button")).find((b) => name.test(b.textContent || b.getAttribute("aria-label") || "")) as HTMLButtonElement | undefined;
const click = async (el: Element | undefined | null) => { expect(el).toBeTruthy(); await act(async () => { (el as HTMLElement).click(); }); await settle(); };
const setValue = async (el: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, value: string) => {
  await act(async () => {
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : el instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value")?.set?.call(el, value);
    el.dispatchEvent(new Event(el instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }));
  });
};
const byLabel = (text: RegExp) => {
  const label = Array.from(container.querySelectorAll("label")).find((l) => text.test(l.textContent || ""));
  expect(label, `label ${text}`).toBeTruthy();
  const id = label!.getAttribute("for");
  return (id ? document.getElementById(id) : label!.querySelector("input,textarea,select")) as HTMLInputElement;
};

beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div"); document.body.append(container); root = createRoot(container);
  saved = vi.fn();
  fetchSpy = vi.fn((url: string) => {
    if (url === "/api/admin/quizzes") return json({ id: "new-quiz" }, 201);
    if (url.startsWith("/api/admin/quizzes/import")) {
      if (url.includes("dry_run")) return json({ draft: { title: "DEMO imported", kind: "practice", settings: { max_attempts: 1, feedback: "after_submit", audience: { type: "all" } }, questions: [{ id: "q1", type: "true_false", prompt: "DEMO?", points: 1, options: [] }] } });
      return json({ id: "imported-1" }, 201);
    }
    return json({});
  });
  vi.stubGlobal("fetch", fetchSpy);
});
afterEach(() => { act(() => root.unmount()); container.remove(); vi.unstubAllGlobals(); });

describe("QuizEditor", () => {
  it("builds a quiz with a live preview and saves it", async () => {
    await act(async () => root.render(<QuizEditor onSaved={saved} />));
    await setValue(byLabel(/^Title/), "DEMO editor quiz");
    await click(button(/Add question/));
    await setValue(byLabel(/Question 1 prompt/), "DEMO which colour?");
    const previewText = container.querySelector('[aria-label="Learner preview"]')?.textContent || "";
    expect(previewText).toContain("DEMO editor quiz");
    expect(previewText).toContain("DEMO which colour?");
    // Correct-answer markers never appear in the learner preview.
    expect(previewText).not.toMatch(/correct/i);
    await setValue(container.querySelector('input[aria-label="Question 1 option 1"]') as HTMLInputElement, "Red");
    await setValue(container.querySelector('input[aria-label="Question 1 option 2"]') as HTMLInputElement, "Blue");
    await click(button(/Save draft/));
    const post = fetchSpy.mock.calls.find(([u, i]) => u === "/api/admin/quizzes" && i?.method === "POST");
    expect(post).toBeTruthy();
    const body = JSON.parse(post![1].body);
    expect(body.title).toBe("DEMO editor quiz");
    expect(body.questions[0]).toMatchObject({ type: "single_choice", prompt: "DEMO which colour?" });
    expect(body.questions[0].options.map((o: { text: string }) => o.text)).toEqual(["Red", "Blue"]);
    expect(saved).toHaveBeenCalledWith("new-quiz");
  });

  it("shows validation errors instead of saving", async () => {
    await act(async () => root.render(<QuizEditor onSaved={saved} />));
    await click(button(/Save draft/));
    expect(container.querySelector('[role="alert"]')?.textContent).toMatch(/Title is required/);
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("study kinds switch feedback to never and explain why", async () => {
    await act(async () => root.render(<QuizEditor onSaved={saved} />));
    await setValue(byLabel(/^Kind/) as unknown as HTMLSelectElement, "follow_up");
    expect((byLabel(/^Show results/) as unknown as HTMLSelectElement).value).toBe("never");
    expect(container.textContent).toMatch(/could cue later recall/i);
  });

  it("locks question editing when attempts exist", async () => {
    const record = { id: "q-1", status: "published", version: 1, attempts: 3, updated_at: "", title: "DEMO", description: "", kind: "practice",
      settings: { max_attempts: 1, feedback: "after_submit", audience: { type: "all" }, shuffle_questions: false, shuffle_options: false, no_going_back: false },
      questions: [{ id: "a", type: "short_answer", prompt: "DEMO", points: 1 }] };
    await act(async () => root.render(<QuizEditor record={record as never} onSaved={saved} />));
    expect(container.textContent).toMatch(/3 attempts/);
    expect(button(/Add question/)?.disabled).toBe(true);
  });
});

describe("ImportPanel", () => {
  it("previews a pasted file then imports and publishes it", async () => {
    await act(async () => root.render(<ImportPanel onImported={saved} />));
    await setValue(container.querySelector("textarea") as HTMLTextAreaElement, "# DEMO imported\n\n## Q1 true_false\nDEMO?\n- [x] True\n- [ ] False\n");
    await click(button(/Check file/));
    expect(fetchSpy.mock.calls[0][0]).toContain("dry_run=1");
    expect(container.textContent).toContain("DEMO imported");
    expect(container.textContent).toMatch(/1 question/);
    await click(button(/Import and publish/));
    const last = fetchSpy.mock.calls.at(-1)!;
    expect(last[0]).toContain("publish=1");
    expect(new Headers(last[1].headers).get("Content-Type")).toBe("text/markdown");
    expect(saved).toHaveBeenCalledWith("imported-1");
  });

  it("shows the API's line-level error", async () => {
    fetchSpy.mockImplementation(() => json({ error: "line 4: unknown question type \"essay\"" }, 422));
    await act(async () => root.render(<ImportPanel onImported={saved} />));
    await setValue(container.querySelector("textarea") as HTMLTextAreaElement, "title: x");
    await click(button(/Check file/));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("line 4");
  });
});
