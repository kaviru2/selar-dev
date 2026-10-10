import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ChatMessage } from "@/lib/api";
import { AnswerFeedback, feedbackState } from "./AnswerFeedback";

const clientFetch = vi.fn();
vi.mock("@/lib/api", () => ({ clientFetch: (...args: unknown[]) => clientFetch(...args) }));

let container: HTMLDivElement;
let root: Root;
beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  clientFetch.mockResolvedValue({});
});
afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.clearAllMocks();
});

function answer(extra: Partial<ChatMessage> = {}): ChatMessage {
  return { id: "m1", thread_id: "t", user_id: "u", role: "assistant", content: "A", status: "complete",
    created_at: "", citations: [], ...extra } as ChatMessage;
}
const button = (label: string) => Array.from(container.querySelectorAll("button"))
  .find((b) => b.getAttribute("aria-label") === label || b.textContent === label) as HTMLButtonElement | undefined;
const posts = () => clientFetch.mock.calls.map(([path, init]) => ({ path, body: JSON.parse(init.body) }));
async function render(message: ChatMessage, onChanged = vi.fn()) {
  await act(async () => root.render(<AnswerFeedback message={message} onChanged={onChanged} />));
  return onChanged;
}
async function type(el: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const proto = el instanceof HTMLInputElement ? HTMLInputElement.prototype : HTMLTextAreaElement.prototype;
  await act(async () => {
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

it("offers exactly one rating question and one report action, with no separate comment box", async () => {
  await render(answer());
  expect(container.textContent).toContain("Was this helpful?");
  expect(button("Helpful")).toBeDefined();
  expect(button("Not helpful")).toBeDefined();
  expect(button("Report a wrong claim")).toBeDefined();
  expect(container.textContent).not.toMatch(/Add comment|Edit comment|Correct answer|Useful\?/);
  expect(container.querySelector("textarea, input")).toBeNull();
});

it("records 👍 as `helpful` without any text and locks the rating", async () => {
  const onChanged = await render(answer());
  await act(async () => button("Helpful")!.click());
  expect(posts()).toEqual([{ path: "/api/chat/messages/m1/feedback", body: { action: "helpful", correction_text: "" } }]);
  expect(onChanged).toHaveBeenCalled();
  await render(answer({ feedback: [{ id: "f", message_id: "m1", action: "helpful", created_at: "" }] }));
  expect(button("Helpful")!.disabled).toBe(true);
  expect(button("Helpful")!.getAttribute("aria-pressed")).toBe("true");
  // A helpful answer can still be downgraded; negative feedback is the safe direction.
  expect(button("Not helpful")!.disabled).toBe(false);
});

it("records 👎 immediately (retraction does not wait for a reason), then offers an optional reason", async () => {
  await render(answer());
  await act(async () => button("Not helpful")!.click());
  expect(posts()[0].body).toEqual({ action: "unhelpful", correction_text: "" });

  const rated = answer({ feedback: [{ id: "f", message_id: "m1", action: "unhelpful", created_at: "" }] });
  await render(rated);
  expect(button("Not helpful")!.disabled).toBe(true);
  expect(button("Helpful")!.disabled).toBe(true);
  expect(container.textContent).toContain("won’t shape your graph or retrieval");
  const input = container.querySelector('input[aria-label^="What was missing"]') as HTMLInputElement;
  expect(input).not.toBeNull();
  expect(button("Add reason")!.disabled).toBe(true);
  await type(input, "Cited the wrong paper");
  await act(async () => container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
  expect(posts()[1].body).toEqual({ action: "unhelpful", correction_text: "Cited the wrong paper" });
});

it("shows a saved 👎 reason with an edit affordance instead of an open box", async () => {
  await render(answer({ feedback: [{ id: "f", message_id: "m1", action: "unhelpful", correction_text: "Too vague", created_at: "" }] }));
  expect(container.textContent).toContain("Reason: Too vague");
  expect(container.querySelector("input")).toBeNull();
  await act(async () => button("Edit")!.click());
  expect((container.querySelector("input") as HTMLInputElement).value).toBe("Too vague");
});

it("reports a wrong claim as one explicit `correction` with required text", async () => {
  await render(answer());
  await act(async () => button("Report a wrong claim")!.click());
  const form = container.querySelector('form[aria-label="Report a wrong claim"]')!;
  expect(form.textContent).toContain("This link is wrong");
  expect(button("Report as wrong")!.disabled).toBe(true);
  await type(form.querySelector("textarea")!, "The baseline was modified, not the original");
  await act(async () => form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
  expect(posts()).toEqual([{ path: "/api/chat/messages/m1/feedback",
    body: { action: "correction", correction_text: "The baseline was modified, not the original" } }]);
});

it("collapses to a single status once an answer has been reported", async () => {
  await render(answer({ status: "superseded", feedback: [{ id: "f", message_id: "m1", action: "correction", correction_text: "x", created_at: "" }],
    graph_update: { concepts_created: 0, concepts_reinforced: 2, links_observed: 0, links_promoted: 0, reducer_version: "v", retracted: true } }));
  expect(container.textContent).toContain("Reported as wrong");
  expect(container.textContent).toContain("Evidence withdrawn");
  expect(container.querySelectorAll("button").length).toBe(0);
});

it("describes 👍 graph effect as citation-concept association, never as a relationship", async () => {
  await render(answer({ feedback: [{ id: "f", message_id: "m1", action: "helpful", created_at: "" }],
    graph_update: { concepts_created: 1, concepts_reinforced: 1, links_observed: 3, links_promoted: 1, reducer_version: "v" } }));
  const link = container.querySelector('a[href="/graph"]')!;
  expect(link.textContent).toContain("associated with 2 concepts (1 new candidate)");
  expect(link.textContent).toContain("never shows that concepts are related");
  expect(container.textContent).not.toMatch(/reinforced|promoted|relationship (found|confirmed)|links? observed/i);
});

it("keeps comments stored on 👍 by the earlier UI visible, read-only", async () => {
  const message = answer({ feedback: [{ id: "f", message_id: "m1", action: "helpful", correction_text: "Great summary", created_at: "" }] });
  expect(feedbackState(message)).toMatchObject({ rating: "helpful", legacyNote: "Great summary", reason: "" });
  await render(message);
  expect(container.textContent).toContain("Your earlier note: Great summary");
  expect(container.querySelector("input, textarea")).toBeNull();
});

it("treats 👍 followed by 👎 as not helpful (negative feedback is sticky)", () => {
  expect(feedbackState(answer({ feedback: [
    { id: "a", message_id: "m1", action: "helpful", created_at: "" },
    { id: "b", message_id: "m1", action: "unhelpful", created_at: "" },
  ] })).rating).toBe("unhelpful");
});

it("surfaces API errors without losing state", async () => {
  clientFetch.mockRejectedValueOnce(new Error("feedback comment must be at most 2000 characters"));
  await render(answer());
  await act(async () => button("Helpful")!.click());
  expect(container.querySelector('[role="alert"]')!.textContent).toContain("at most 2000");
  expect(button("Helpful")!.disabled).toBe(false);
});
