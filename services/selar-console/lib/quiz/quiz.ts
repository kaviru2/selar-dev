// lib/quiz/quiz.ts — types and pure helpers for the quiz console (issue #86).
// Rules that matter for integrity live in the Go API; these helpers only shape
// what the UI shows.

export type QuizKind = "initial" | "follow_up" | "practice";
export type QuizStatus = "draft" | "published" | "closed";
export type FeedbackPolicy = "never" | "after_submit" | "after_close";
export type QuestionType = "single_choice" | "multiple_choice" | "true_false" | "short_answer" | "free_recall" | "cued_recall";
export type AvailabilityState = "hidden" | "upcoming" | "available" | "in_progress" | "completed" | "missed";

export const QUESTION_TYPES: { value: QuestionType; label: string }[] = [
  { value: "single_choice", label: "Single choice" },
  { value: "multiple_choice", label: "Multiple choice" },
  { value: "true_false", label: "True / false" },
  { value: "short_answer", label: "Short answer" },
  { value: "cued_recall", label: "Cued recall" },
  { value: "free_recall", label: "Free recall" },
];

export const KIND_LABEL: Record<QuizKind, string> = { initial: "Initial test", follow_up: "Follow-up test", practice: "Practice" };

export const isChoice = (t: QuestionType) => t === "single_choice" || t === "multiple_choice" || t === "true_false";

export interface Availability {
  state: AvailabilityState;
  opens_at?: string;
  closes_at?: string;
  reason?: string;
  attempts_used: number;
  attempts_allowed: number;
}

export interface LearnerQuizCard {
  id: string;
  title: string;
  description: string;
  kind: QuizKind;
  question_count: number;
  time_limit_seconds?: number;
  no_going_back: boolean;
  availability: Availability;
  open_attempt_id?: string;
  last_attempt?: {
    id: string;
    submitted_at?: string;
    score?: number | null;
    max_points?: number;
    pending_review?: number;
    result_visible: boolean;
  };
}

export interface LearnerQuestion {
  id: string;
  index: number;
  type: QuestionType;
  prompt: string;
  cue?: string;
  points: number;
  options?: { id: string; text: string }[];
}

export interface SavedAnswer {
  selected: string[];
  text: string;
}

export interface AttemptView {
  attempt_id: string;
  quiz_id: string;
  title: string;
  description: string;
  kind: QuizKind;
  started_at: string;
  deadline_at?: string;
  server_now: string;
  no_going_back: boolean;
  total: number;
  current_position: number;
  questions: LearnerQuestion[];
  answers: Record<string, SavedAnswer>;
}

export interface Summary {
  max_points: number;
  auto_points: number;
  pending_review: number;
  score?: number | null;
  percent?: number | null;
}

export interface ResultQuestion {
  id: string;
  index: number;
  type: QuestionType;
  prompt: string;
  cue?: string;
  options?: { id: string; text: string; correct: boolean; selected: boolean }[];
  text?: string;
  accepted_answers?: string[];
  points: number;
  score?: number | null;
  correct?: boolean | null;
  pending_review: boolean;
  explanation?: string;
  grader_note?: string;
  source_document?: string;
  source_url?: string;
}

export interface AttemptResult {
  attempt_id: string;
  quiz_id: string;
  title: string;
  submitted_at: string;
  submit_reason: "learner" | "time_limit" | "closed";
  feedback_available: boolean;
  feedback_policy: FeedbackPolicy;
  summary?: Summary;
  questions?: ResultQuestion[];
}

// ---------- admin draft ----------

export interface DraftOption {
  id: string;
  text: string;
  correct: boolean;
}

export interface DraftQuestion {
  id: string;
  type: QuestionType;
  prompt: string;
  cue: string;
  options: DraftOption[];
  accepted_answers: string[];
  points: number;
  rubric: string;
  explanation: string;
  source_document: string;
  source_url: string;
}

export interface RelativeWindow {
  anchor: "first_reading" | "document" | "quiz_submitted";
  days: number;
  window_days?: number;
  document?: string;
  quiz_id?: string;
}

export interface QuizSettings {
  open_at?: string | null;
  close_at?: string | null;
  after?: RelativeWindow | null;
  time_limit_seconds?: number;
  max_attempts: number;
  feedback: FeedbackPolicy;
  audience: { type: "all" | "cohort" | "group"; value?: string };
  shuffle_questions: boolean;
  shuffle_options: boolean;
  no_going_back: boolean;
}

export interface QuizDraft {
  title: string;
  description: string;
  kind: QuizKind;
  settings: QuizSettings;
  questions: DraftQuestion[];
}

export interface QuizRecord extends QuizDraft {
  id: string;
  status: QuizStatus;
  version: number;
  attempts: number;
  updated_at: string;
}

export interface AdminQuizSummary {
  id: string;
  title: string;
  kind: QuizKind;
  status: QuizStatus;
  question_count: number;
  attempts: number;
  submitted: number;
  pending_review: number;
  updated_at: string;
  settings: QuizSettings;
}

// ---------- learner list ----------

export function groupCards(cards: LearnerQuizCard[]) {
  const rank = (c: LearnerQuizCard) => (c.availability.state === "in_progress" ? 0 : 1);
  return {
    available: cards.filter((c) => c.availability.state === "available" || c.availability.state === "in_progress").sort((a, b) => rank(a) - rank(b)),
    upcoming: cards.filter((c) => c.availability.state === "upcoming"),
    completed: cards.filter((c) => c.availability.state === "completed" || c.availability.state === "missed"),
  };
}

export function relative(target: Date, now: Date): string {
  const ms = target.getTime() - now.getTime();
  const abs = Math.abs(ms);
  const units: [number, string][] = [[86_400_000, "day"], [3_600_000, "hour"], [60_000, "minute"]];
  for (const [size, name] of units) {
    if (abs >= size) {
      const n = Math.round(abs / size);
      return `${n} ${name}${n === 1 ? "" : "s"}`;
    }
  }
  return "less than a minute";
}

const dateFmt = (iso: string) =>
  new Date(iso).toLocaleString(undefined, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });

export function describeAvailability(card: LearnerQuizCard, now = new Date()): string {
  const a = card.availability;
  switch (a.state) {
    case "upcoming":
      if (a.opens_at) return `Opens in ${relative(new Date(a.opens_at), now)} (${dateFmt(a.opens_at)})`;
      return a.reason || "Not open yet";
    case "available":
    case "in_progress":
      return a.closes_at ? `Closes in ${relative(new Date(a.closes_at), now)} (${dateFmt(a.closes_at)})` : "Open now";
    case "missed":
      return "Closed before you took it";
    case "completed":
      return "Completed";
    default:
      return "";
  }
}

// ---------- attempt ----------

export function formatDuration(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

/**
 * Remaining time anchored on the server clock: serverNow was observed at
 * client time loadedAt (performance.now()); nowMono is the current performance.now().
 */
export function remainingMs(deadline: string | undefined, serverNow: string, loadedAt: number, nowMono: number): number | null {
  if (!deadline) return null;
  const elapsed = nowMono - loadedAt;
  return new Date(deadline).getTime() - (new Date(serverNow).getTime() + elapsed);
}

export function answerIsEmpty(a: SavedAnswer | undefined): boolean {
  return !a || ((a.selected?.length ?? 0) === 0 && (a.text ?? "").trim() === "");
}

export function progress(answers: Record<string, SavedAnswer>, total: number) {
  const answered = Object.values(answers).filter((a) => !answerIsEmpty(a)).length;
  return { answered, total, percent: total ? Math.round((answered / total) * 100) : 0 };
}

// ---------- editor ----------

export function emptyDraft(): QuizDraft {
  return {
    title: "",
    description: "",
    kind: "practice",
    settings: {
      max_attempts: 1,
      feedback: "after_submit",
      audience: { type: "all" },
      shuffle_questions: false,
      shuffle_options: false,
      no_going_back: false,
    },
    questions: [],
  };
}

export function newQuestion(type: QuestionType): DraftQuestion {
  const base: DraftQuestion = {
    id: "", type, prompt: "", cue: "", options: [], accepted_answers: [], points: type === "free_recall" ? 5 : 1,
    rubric: "", explanation: "", source_document: "", source_url: "",
  };
  if (type === "true_false") {
    base.options = [{ id: "true", text: "True", correct: true }, { id: "false", text: "False", correct: false }];
  } else if (isChoice(type)) {
    base.options = [{ id: "o1", text: "", correct: true }, { id: "o2", text: "", correct: false }];
  }
  return base;
}

export function nextOptionId(options: DraftOption[]): string {
  const used = new Set(options.map((o) => o.id));
  let i = options.length + 1;
  while (used.has(`o${i}`)) i++;
  return `o${i}`;
}

export function moveQuestion(qs: DraftQuestion[], index: number, delta: number): DraftQuestion[] {
  const target = index + delta;
  if (target < 0 || target >= qs.length) return qs;
  const out = [...qs];
  [out[index], out[target]] = [out[target], out[index]];
  return out;
}

export function duplicateQuestion(qs: DraftQuestion[], index: number): DraftQuestion[] {
  const copy: DraftQuestion = structuredClone(qs[index]);
  copy.id = "";
  return [...qs.slice(0, index + 1), copy, ...qs.slice(index + 1)];
}

/** Client-side checks mirroring the API's validation, for instant feedback. */
export function validateDraft(d: QuizDraft): string[] {
  const errors: string[] = [];
  if (!d.title.trim()) errors.push("Title is required");
  const s = d.settings;
  if (s.open_at && s.close_at && new Date(s.close_at) <= new Date(s.open_at)) errors.push("Close time must be after open time");
  if (s.audience.type !== "all" && !s.audience.value?.trim()) errors.push("Choose a cohort or group for the audience");
  if (s.after?.anchor === "document" && !s.after.document?.trim()) errors.push("Relative window: name the document");
  if (s.after?.anchor === "quiz_submitted" && !s.after.quiz_id) errors.push("Relative window: choose the earlier quiz");
  if (d.questions.length === 0) errors.push("Add at least one question");
  d.questions.forEach((q, i) => {
    const n = `Question ${i + 1}`;
    if (!q.prompt.trim()) errors.push(`${n}: prompt is required`);
    if (q.points < 0 || Number.isNaN(q.points)) errors.push(`${n}: points must be zero or more`);
    if (isChoice(q.type)) {
      if (q.options.some((o) => !o.text.trim())) errors.push(`${n}: option text must not be empty`);
      const correct = q.options.filter((o) => o.correct).length;
      if (q.options.length < 2) errors.push(`${n}: add at least two options`);
      if (q.type === "multiple_choice" && correct === 0) errors.push(`${n}: mark at least one correct option`);
      if (q.type !== "multiple_choice" && correct !== 1) errors.push(`${n}: mark exactly one correct option`);
    }
  });
  return errors;
}

/** Convert an RFC 3339 instant to the value of a datetime-local input (local time). */
export function toLocalInput(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** Convert a datetime-local value (browser local time) to RFC 3339 UTC. */
export function fromLocalInput(v: string): string | null {
  if (!v) return null;
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}
