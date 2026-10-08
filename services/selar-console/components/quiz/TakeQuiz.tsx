"use client";

// TakeQuiz — the learner attempt flow. The server owns the deadline, the
// no-going-back position and submission; this component autosaves and renders.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  answerIsEmpty, formatDuration, progress, remainingMs,
  type AttemptView, type LearnerQuestion, type SavedAnswer,
} from "@/lib/quiz/quiz";

type SaveState = "idle" | "saving" | "saved" | "error";
const TEXT_DEBOUNCE_MS = 1200;

class AttemptOver extends Error {}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { ...init, headers: { "Content-Type": "application/json", ...(init?.headers || {}) } });
  if (res.status === 409) {
    const body = await res.json().catch(() => ({}));
    const msg = String(body.error || "");
    if (/submitted|closed|time|deadline|not available/i.test(msg)) throw new AttemptOver(msg);
    throw new Error(msg || "conflict");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}

function Confirm({ title, body, confirm, onConfirm, onCancel, cancel = "Keep working" }: {
  title: string; body: React.ReactNode; confirm: string; cancel?: string; onConfirm: () => void; onCancel: () => void;
}) {
  const ref = useRef<HTMLButtonElement>(null);
  useEffect(() => { ref.current?.focus(); }, []);
  return (
    <div className="quiz-modal-backdrop" onKeyDown={(e) => { if (e.key === "Escape") onCancel(); }}>
      <div className="quiz-modal" role="alertdialog" aria-modal="true" aria-labelledby="quiz-confirm-title" aria-describedby="quiz-confirm-body">
        <h2 id="quiz-confirm-title">{title}</h2>
        <div id="quiz-confirm-body">{body}</div>
        <div className="quiz-modal-actions">
          <button type="button" className="ui-btn ui-btn--ghost" onClick={onCancel}>{cancel}</button>
          <button type="button" className="ui-btn ui-btn--primary" ref={ref} onClick={onConfirm}>{confirm}</button>
        </div>
      </div>
    </div>
  );
}

function QuestionInput({ q, value, onChoice, onText }: {
  q: LearnerQuestion; value: SavedAnswer | undefined;
  onChoice: (selected: string[]) => void; onText: (text: string) => void;
}) {
  const selected = value?.selected ?? [];
  const name = `q-${q.id}`;
  if (q.type === "single_choice" || q.type === "true_false" || q.type === "multiple_choice") {
    const multi = q.type === "multiple_choice";
    return (
      <fieldset className="quiz-options">
        <legend className="sr-only">{multi ? "Choose all that apply" : "Choose one"}</legend>
        {multi && <p className="quiz-hint">Choose all that apply.</p>}
        {(q.options ?? []).map((o) => {
          const checked = selected.includes(o.id);
          return (
            <label key={o.id} className={`quiz-option${checked ? " is-checked" : ""}`}>
              <input
                type={multi ? "checkbox" : "radio"}
                name={name}
                value={o.id}
                checked={checked}
                onChange={() => onChoice(multi ? (checked ? selected.filter((s) => s !== o.id) : [...selected, o.id]) : [o.id])}
              />
              <span>{o.text}</span>
            </label>
          );
        })}
      </fieldset>
    );
  }
  const long = q.type === "free_recall";
  return (
    <div className="quiz-text">
      <label htmlFor={`${name}-text`} className="sr-only">Your answer</label>
      <textarea
        id={`${name}-text`}
        rows={long ? 10 : 3}
        value={value?.text ?? ""}
        placeholder={long ? "Write everything you remember." : "Your answer"}
        onChange={(e) => onText(e.target.value)}
      />
    </div>
  );
}

export function TakeQuiz({ initial, onDone }: { initial: AttemptView; onDone: (attemptId: string) => void }) {
  const [view, setView] = useState(initial);
  const [answers, setAnswers] = useState<Record<string, SavedAnswer>>(initial.answers ?? {});
  const [index, setIndex] = useState(0);
  const [save, setSave] = useState<SaveState>("idle");
  const [dialog, setDialog] = useState<"submit" | "advance" | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Remaining time, recomputed from the server clock each second (null = no limit).
  const [remaining, setRemaining] = useState<number | null>(() => (initial.deadline_at ? Date.parse(initial.deadline_at) - Date.parse(initial.server_now) : null));
  const shownAt = useRef(0);
  const answersRef = useRef(initial.answers ?? {});
  useEffect(() => { answersRef.current = answers; }, [answers]);
  useEffect(() => { shownAt.current = performance.now(); }, []);
  const pending = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());
  const finished = useRef(false);
  const headingRef = useRef<HTMLHeadingElement>(null);

  const id = view.attempt_id;
  const questions = view.questions;
  const q = questions[Math.min(index, questions.length - 1)];
  const position = view.no_going_back ? view.current_position : index;
  const isLast = position >= view.total - 1;
  const prog = progress(answers, view.total);

  const finish = useCallback(() => {
    if (finished.current) return;
    finished.current = true;
    pending.current.forEach((t) => clearTimeout(t));
    onDone(id);
  }, [id, onDone]);

  const persist = useCallback(async (questionId: string, a: SavedAnswer) => {
    const spent = Math.round(performance.now() - shownAt.current);
    shownAt.current = performance.now();
    setSave("saving");
    try {
      await call(`/api/quiz-attempts/${id}/answers`, {
        method: "PUT",
        body: JSON.stringify({ question_id: questionId, selected: a.selected, text: a.text, time_on_question_ms: spent }),
      });
      setSave("saved");
    } catch (e) {
      if (e instanceof AttemptOver) return finish();
      setSave("error");
    }
  }, [id, finish]);

  const flush = useCallback(async () => {
    const entries = Array.from(pending.current.entries());
    pending.current.clear();
    for (const [qid, t] of entries) {
      clearTimeout(t);
      await persist(qid, answersRef.current[qid] ?? { selected: [], text: "" });
    }
  }, [persist]);


  const setChoice = (selected: string[]) => {
    const a = { selected, text: "" };
    setAnswers((prev) => ({ ...prev, [q.id]: a }));
    void persist(q.id, a);
  };
  const setText = (text: string) => {
    const a = { selected: [], text };
    setAnswers((prev) => ({ ...prev, [q.id]: a }));
    setSave("idle");
    const old = pending.current.get(q.id);
    if (old) clearTimeout(old);
    pending.current.set(q.id, setTimeout(() => {
      pending.current.delete(q.id);
      void persist(q.id, answersRef.current[q.id] ?? a);
    }, TEXT_DEBOUNCE_MS));
  };

  // Countdown anchored on server time, not the device clock.
  useEffect(() => {
    if (!view.deadline_at) return;
    const loadedAt = performance.now();
    const tick = () => setRemaining(remainingMs(view.deadline_at, view.server_now, loadedAt, performance.now()));
    tick();
    const t = setInterval(tick, 1000);
    return () => clearInterval(t);
  }, [view.deadline_at, view.server_now]);
  useEffect(() => {
    if (remaining !== null && remaining <= 0) {
      // Server auto-submits at the deadline; make sure our last text lands first.
      void flush().finally(finish);
    }
  }, [remaining, flush, finish]);

  // Save on tab hide so a closed laptop does not lose a paragraph.
  useEffect(() => {
    const onHide = () => { if (document.visibilityState === "hidden") void flush(); };
    document.addEventListener("visibilitychange", onHide);
    return () => document.removeEventListener("visibilitychange", onHide);
  }, [flush]);

  useEffect(() => { headingRef.current?.focus(); }, [q?.id]);

  const go = async (delta: number) => {
    await flush();
    shownAt.current = performance.now();
    setIndex((i) => Math.max(0, Math.min(questions.length - 1, i + delta)));
  };

  const advance = async () => {
    setDialog(null);
    setBusy(true);
    try {
      await flush();
      const next = await call<AttemptView>(`/api/quiz-attempts/${id}/advance`, { method: "POST" });
      setView(next);
      setAnswers((prev) => ({ ...prev, ...(next.answers ?? {}) }));
      setIndex(0);
      shownAt.current = performance.now();
    } catch (e) {
      if (e instanceof AttemptOver) return finish();
      setError(e instanceof Error ? e.message : "Could not continue");
    } finally {
      setBusy(false);
    }
  };

  const submit = async () => {
    setDialog(null);
    setBusy(true);
    try {
      await flush();
      await call(`/api/quiz-attempts/${id}/submit`, { method: "POST" });
      finish();
    } catch (e) {
      if (e instanceof AttemptOver) return finish();
      setError(e instanceof Error ? e.message : "Could not submit");
      setBusy(false);
    }
  };

  const unanswered = view.total - prog.answered;
  const saveLabel = useMemo(() => ({ idle: "", saving: "Saving…", saved: "Saved", error: "Not saved. Check your connection." })[save], [save]);

  if (!q) return null;

  return (
    <div className="quiz-take">
      <header className="quiz-take-head">
        <div>
          <p className="quiz-take-title">{view.title}</p>
          <p className="quiz-take-pos">Question {position + 1} of {view.total}</p>
        </div>
        {remaining !== null && (
          <p className={`quiz-timer${remaining < 60_000 ? " is-low" : ""}`} role="timer" aria-live={remaining < 60_000 ? "assertive" : "off"}>
            <span className="sr-only">Time left </span>{formatDuration(remaining)}
          </p>
        )}
      </header>

      <div className="quiz-progress" role="progressbar" aria-valuemin={0} aria-valuemax={view.total} aria-valuenow={prog.answered} aria-label="Answered">
        <span style={{ width: `${prog.percent}%` }} />
      </div>
      <p className="quiz-progress-text">{prog.answered} of {view.total} answered <span className="quiz-save" aria-live="polite">{saveLabel}</span></p>

      {view.no_going_back && <p className="quiz-note">This is a recall test. Once you move on you cannot return to a question.</p>}

      <section className="quiz-question" aria-labelledby={`qh-${q.id}`}>
        <h2 id={`qh-${q.id}`} tabIndex={-1} ref={headingRef}>{q.prompt}</h2>
        {q.cue && <p className="quiz-cue"><span>Cue:</span> {q.cue}</p>}
        <QuestionInput q={q} value={answers[q.id]} onChoice={setChoice} onText={setText} />
      </section>

      {error && <p role="alert" className="quiz-error">{error}</p>}

      <nav className="quiz-nav" aria-label="Question navigation">
        {!view.no_going_back && (
          <button type="button" className="ui-btn ui-btn--ghost" disabled={index === 0 || busy} onClick={() => go(-1)}>Previous</button>
        )}
        <span className="quiz-nav-spacer" />
        {!isLast && (
          <button type="button" className="ui-btn" disabled={busy} onClick={() => (view.no_going_back ? setDialog("advance") : go(1))}>Next</button>
        )}
        {(isLast || !view.no_going_back) && (
          <button type="button" className={`ui-btn ${isLast ? "ui-btn--primary" : "ui-btn--ghost"}`} disabled={busy} onClick={() => setDialog("submit")}>Submit</button>
        )}
      </nav>

      {!view.no_going_back && view.total > 1 && (
        <ol className="quiz-jump" aria-label="Jump to question">
          {questions.map((qq, i) => (
            <li key={qq.id}>
              <button type="button" aria-current={i === index ? "step" : undefined} className={answerIsEmpty(answers[qq.id]) ? "" : "is-answered"} onClick={() => go(i - index)}>
                <span className="sr-only">Question </span>{i + 1}{answerIsEmpty(answers[qq.id]) ? <span className="sr-only"> (unanswered)</span> : null}
              </button>
            </li>
          ))}
        </ol>
      )}

      {dialog === "submit" && (
        <Confirm
          title="Submit your answers?"
          body={<>
            {unanswered > 0 && <p><strong>{unanswered} question{unanswered === 1 ? " is" : "s are"} unanswered.</strong></p>}
            <p>You cannot change your answers after submitting.</p>
          </>}
          confirm="Yes, submit"
          onConfirm={submit}
          onCancel={() => setDialog(null)}
        />
      )}
      {dialog === "advance" && (
        <Confirm
          title="Move to the next question?"
          body={<p>You cannot come back to this question{answerIsEmpty(answers[q.id]) ? ", and it has no answer yet" : ""}.</p>}
          confirm="Yes, continue"
          cancel="Stay here"
          onConfirm={advance}
          onCancel={() => setDialog(null)}
        />
      )}
    </div>
  );
}
