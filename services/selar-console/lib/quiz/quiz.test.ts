import { describe, expect, it } from "vitest";
import {
  groupCards,
  describeAvailability,
  formatDuration,
  remainingMs,
  answerIsEmpty,
  progress,
  emptyDraft,
  newQuestion,
  moveQuestion,
  duplicateQuestion,
  validateDraft,
  type LearnerQuizCard,
  type DraftQuestion,
} from "./quiz";

const card = (id: string, state: LearnerQuizCard["availability"]["state"], extra: Partial<LearnerQuizCard> = {}): LearnerQuizCard => ({
  id, title: id, description: "", kind: "practice", question_count: 3, no_going_back: false,
  availability: { state, attempts_used: 0, attempts_allowed: 1 }, ...extra,
});

describe("learner quiz list", () => {
  it("groups cards into available, upcoming and completed sections", () => {
    const g = groupCards([card("a", "available"), card("b", "in_progress"), card("c", "upcoming"), card("d", "completed"), card("e", "missed")]);
    expect(g.available.map((c) => c.id)).toEqual(["b", "a"]);
    expect(g.upcoming.map((c) => c.id)).toEqual(["c"]);
    expect(g.completed.map((c) => c.id)).toEqual(["d", "e"]);
  });

  it("describes when a quiz opens or why it has no date yet", () => {
    const now = new Date("2026-10-08T10:00:00Z");
    expect(describeAvailability(card("x", "upcoming", { availability: { state: "upcoming", reason: "Opens after your first reading session", attempts_used: 0, attempts_allowed: 1 } }), now))
      .toBe("Opens after your first reading session");
    expect(describeAvailability(card("x", "upcoming", { availability: { state: "upcoming", opens_at: "2026-10-10T10:00:00Z", attempts_used: 0, attempts_allowed: 1 } }), now))
      .toMatch(/^Opens in 2 days/);
    expect(describeAvailability(card("x", "available", { availability: { state: "available", closes_at: "2026-10-08T13:00:00Z", attempts_used: 0, attempts_allowed: 1 } }), now))
      .toMatch(/^Closes in 3 hours/);
    expect(describeAvailability(card("x", "missed"), now)).toBe("Closed before you took it");
  });
});

describe("attempt helpers", () => {
  it("formats durations and remaining time", () => {
    expect(formatDuration(65_000)).toBe("1:05");
    expect(formatDuration(3_725_000)).toBe("1:02:05");
    expect(formatDuration(-5)).toBe("0:00");
    // Server time anchors the countdown, not the device clock.
    const deadline = "2026-10-08T10:10:00Z";
    expect(remainingMs(deadline, "2026-10-08T10:00:00Z", 1000, 61_000)).toBe(540_000);
    expect(remainingMs(undefined, "2026-10-08T10:00:00Z", 0, 0)).toBeNull();
  });

  it("knows when an answer is empty and computes progress", () => {
    expect(answerIsEmpty(undefined)).toBe(true);
    expect(answerIsEmpty({ selected: [], text: "  " })).toBe(true);
    expect(answerIsEmpty({ selected: ["o1"], text: "" })).toBe(false);
    expect(progress({ q1: { selected: ["o1"], text: "" }, q2: { selected: [], text: "" } }, 4)).toEqual({ answered: 1, total: 4, percent: 25 });
  });
});

describe("admin quiz editor", () => {
  it("creates sensible new questions per type", () => {
    expect(newQuestion("true_false").options.map((o) => o.id)).toEqual(["true", "false"]);
    expect(newQuestion("single_choice").options).toHaveLength(2);
    expect(newQuestion("free_recall").options).toEqual([]);
    expect(emptyDraft().settings.feedback).toBe("after_submit");
  });

  it("reorders and duplicates questions without sharing references", () => {
    const qs: DraftQuestion[] = [newQuestion("free_recall"), newQuestion("short_answer"), newQuestion("cued_recall")];
    qs[0].prompt = "A"; qs[1].prompt = "B"; qs[2].prompt = "C";
    expect(moveQuestion(qs, 2, -1).map((q) => q.prompt)).toEqual(["A", "C", "B"]);
    expect(moveQuestion(qs, 0, -1).map((q) => q.prompt)).toEqual(["A", "B", "C"]);
    const dup = duplicateQuestion(qs, 1);
    expect(dup.map((q) => q.prompt)).toEqual(["A", "B", "B", "C"]);
    dup[2].prompt = "changed";
    expect(dup[1].prompt).toBe("B");
    expect(dup[2].id).toBe("");
  });

  it("validates drafts with question numbers in messages", () => {
    const d = emptyDraft();
    expect(validateDraft(d)).toContain("Title is required");
    d.title = "DEMO";
    expect(validateDraft(d)).toContain("Add at least one question");
    const q = newQuestion("single_choice");
    q.prompt = "Pick";
    q.options = [{ id: "o1", text: "A", correct: false }, { id: "o2", text: "B", correct: false }];
    d.questions = [q];
    expect(validateDraft(d)).toEqual(["Question 1: mark exactly one correct option"]);
    q.options[1].correct = true;
    expect(validateDraft(d)).toEqual([]);
    d.settings.open_at = "2026-10-10T10:00:00Z";
    d.settings.close_at = "2026-10-09T10:00:00Z";
    expect(validateDraft(d)).toContain("Close time must be after open time");
  });
});
