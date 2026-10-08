"use client";

// QuizEditor — create/edit a quiz with a live learner preview.
// The API re-validates everything; client checks are for fast feedback only.

import { useId, useMemo, useState } from "react";
import { Button } from "@/components/ui/Button";
import { quizApi } from "@/lib/quiz/client";
import {
  KIND_LABEL, QUESTION_TYPES, duplicateQuestion, emptyDraft, fromLocalInput, isChoice, moveQuestion,
  newQuestion, nextOptionId, toLocalInput, validateDraft,
  type DraftQuestion, type FeedbackPolicy, type QuestionType, type QuizDraft, type QuizKind, type QuizRecord, type QuizSettings,
} from "@/lib/quiz/quiz";

type RawQuestion = Partial<DraftQuestion> & { type: QuestionType; prompt: string };

function normalise(q: RawQuestion): DraftQuestion {
  return {
    id: q.id ?? "", type: q.type, prompt: q.prompt ?? "", cue: q.cue ?? "",
    options: (q.options ?? []).map((o) => ({ id: o.id, text: o.text, correct: !!o.correct })),
    accepted_answers: q.accepted_answers ?? [], points: q.points ?? 1, rubric: q.rubric ?? "",
    explanation: q.explanation ?? "", source_document: q.source_document ?? "", source_url: q.source_url ?? "",
  };
}

function fromRecord(r?: QuizRecord): QuizDraft {
  if (!r) return emptyDraft();
  const base = emptyDraft();
  return {
    title: r.title, description: r.description ?? "", kind: r.kind,
    settings: { ...base.settings, ...r.settings, audience: r.settings.audience ?? { type: "all" } },
    questions: (r.questions as RawQuestion[]).map(normalise),
  };
}

/** Strip empty optional fields so the API's JSON stays tidy. */
function toPayload(d: QuizDraft): QuizDraft {
  return {
    ...d,
    questions: d.questions.map((q) => ({
      ...q,
      options: isChoice(q.type) ? q.options : [],
      accepted_answers: q.type === "short_answer" || q.type === "cued_recall" ? q.accepted_answers.filter((a) => a.trim()) : [],
    })),
  };
}

const FEEDBACK_LABEL: Record<FeedbackPolicy, string> = {
  never: "Never",
  after_submit: "After the learner submits",
  after_close: "After the quiz closes",
};

function Field({ label, children, hint, id }: { label: string; children: React.ReactNode; hint?: React.ReactNode; id: string }) {
  return (
    <div className="aq-field">
      <label htmlFor={id}>{label}</label>
      {children}
      {hint && <p className="aq-hint">{hint}</p>}
    </div>
  );
}

function QuestionCard({ q, i, count, locked, onChange, onMove, onDuplicate, onRemove }: {
  q: DraftQuestion; i: number; count: number; locked: boolean;
  onChange: (q: DraftQuestion) => void; onMove: (d: number) => void; onDuplicate: () => void; onRemove: () => void;
}) {
  const uid = useId();
  const n = i + 1;
  const set = (patch: Partial<DraftQuestion>) => onChange({ ...q, ...patch });
  const setCorrect = (id: string) => set({
    options: q.options.map((o) => q.type === "multiple_choice" ? (o.id === id ? { ...o, correct: !o.correct } : o) : { ...o, correct: o.id === id }),
  });
  return (
    <li className="aq-q">
      <fieldset disabled={locked}>
        <legend className="aq-q-head">
          <span className="aq-q-num">Q{n}</span>
          <select aria-label={`Question ${n} type`} value={q.type} onChange={(e) => {
            const t = e.target.value as QuestionType;
            const fresh = newQuestion(t);
            onChange({ ...q, type: t, options: isChoice(t) ? (isChoice(q.type) && t !== "true_false" && q.type !== "true_false" ? q.options : fresh.options) : [] });
          }}>
            {QUESTION_TYPES.map((t) => <option key={t.value} value={t.value}>{t.label}</option>)}
          </select>
          <span className="aq-q-tools">
            <button type="button" aria-label={`Move question ${n} up`} disabled={i === 0} onClick={() => onMove(-1)}>↑</button>
            <button type="button" aria-label={`Move question ${n} down`} disabled={i === count - 1} onClick={() => onMove(1)}>↓</button>
            <button type="button" aria-label={`Duplicate question ${n}`} onClick={onDuplicate}>⧉</button>
            <button type="button" aria-label={`Delete question ${n}`} onClick={onRemove}>✕</button>
          </span>
        </legend>
        <Field id={`${uid}-p`} label={`Question ${n} prompt`}>
          <textarea id={`${uid}-p`} rows={2} value={q.prompt} onChange={(e) => set({ prompt: e.target.value })} />
        </Field>
        {q.type === "cued_recall" && (
          <Field id={`${uid}-c`} label="Cue shown to the learner">
            <input id={`${uid}-c`} value={q.cue} onChange={(e) => set({ cue: e.target.value })} />
          </Field>
        )}
        {isChoice(q.type) && (
          <div className="aq-options" role="group" aria-label={`Question ${n} options`}>
            {q.options.map((o, oi) => (
              <div className="aq-option" key={o.id}>
                <input
                  type={q.type === "multiple_choice" ? "checkbox" : "radio"}
                  name={`${uid}-correct`}
                  checked={o.correct}
                  aria-label={`Mark option ${oi + 1} correct`}
                  onChange={() => setCorrect(o.id)}
                />
                <input
                  aria-label={`Question ${n} option ${oi + 1}`}
                  value={o.text}
                  readOnly={q.type === "true_false"}
                  onChange={(e) => set({ options: q.options.map((x) => (x.id === o.id ? { ...x, text: e.target.value } : x)) })}
                />
                {q.type !== "true_false" && q.options.length > 2 && (
                  <button type="button" aria-label={`Remove option ${oi + 1}`} onClick={() => set({ options: q.options.filter((x) => x.id !== o.id) })}>✕</button>
                )}
              </div>
            ))}
            {q.type !== "true_false" && (
              <button type="button" className="aq-link" onClick={() => set({ options: [...q.options, { id: nextOptionId(q.options), text: "", correct: false }] })}>+ Add option</button>
            )}
          </div>
        )}
        {(q.type === "short_answer" || q.type === "cued_recall") && (
          <Field id={`${uid}-a`} label="Accepted answers (one per line)" hint={q.type === "cued_recall" ? "Optional. Recall answers are always scored by a person." : "Exact match after ignoring case, spaces and punctuation. Others go to manual review."}>
            <textarea id={`${uid}-a`} rows={2} value={q.accepted_answers.join("\n")} onChange={(e) => set({ accepted_answers: e.target.value.split("\n") })} />
          </Field>
        )}
        <div className="aq-grid2">
          <Field id={`${uid}-pts`} label="Points">
            <input id={`${uid}-pts`} type="number" min={0} step={0.5} value={q.points} onChange={(e) => set({ points: Number(e.target.value) })} />
          </Field>
          <Field id={`${uid}-src`} label="Source document (optional)">
            <input id={`${uid}-src`} value={q.source_document} onChange={(e) => set({ source_document: e.target.value })} />
          </Field>
        </div>
        <details className="aq-more">
          <summary>Rubric, explanation and link</summary>
          <Field id={`${uid}-r`} label="Rubric notes (graders only)">
            <textarea id={`${uid}-r`} rows={2} value={q.rubric} onChange={(e) => set({ rubric: e.target.value })} />
          </Field>
          <Field id={`${uid}-e`} label="Explanation (shown only when results are visible)">
            <textarea id={`${uid}-e`} rows={2} value={q.explanation} onChange={(e) => set({ explanation: e.target.value })} />
          </Field>
          <Field id={`${uid}-u`} label="Source link (optional)">
            <input id={`${uid}-u`} type="url" value={q.source_url} onChange={(e) => set({ source_url: e.target.value })} />
          </Field>
        </details>
      </fieldset>
    </li>
  );
}

function Preview({ d }: { d: QuizDraft }) {
  return (
    <aside className="aq-preview" aria-label="Learner preview">
      <p className="aq-preview-tag">Learner preview</p>
      <h3>{d.title || "Untitled quiz"}</h3>
      {d.description && <p className="aq-muted">{d.description}</p>}
      <p className="aq-muted">
        {d.questions.length} question{d.questions.length === 1 ? "" : "s"}
        {d.settings.time_limit_seconds ? ` · ${Math.round(d.settings.time_limit_seconds / 60)} min` : ""}
        {d.settings.no_going_back ? " · one at a time, no going back" : ""}
      </p>
      <ol className="aq-preview-qs">
        {d.questions.map((q, i) => (
          <li key={i}>
            <p className="aq-preview-prompt">{q.prompt || <em>Question {i + 1}</em>}</p>
            {q.type === "cued_recall" && q.cue && <p className="quiz-cue"><span>Cue:</span> {q.cue}</p>}
            {isChoice(q.type) ? (
              <ul className="aq-preview-opts">
                {q.options.map((o) => (
                  <li key={o.id}><span className={q.type === "multiple_choice" ? "aq-box" : "aq-dot"} aria-hidden="true" />{o.text || "…"}</li>
                ))}
              </ul>
            ) : (
              <div className={`aq-preview-text${q.type === "free_recall" ? " is-long" : ""}`} aria-hidden="true" />
            )}
          </li>
        ))}
      </ol>
    </aside>
  );
}

export function QuizEditor({ record, onSaved }: { record?: QuizRecord; onSaved: (id: string) => void }) {
  const [d, setD] = useState<QuizDraft>(() => fromRecord(record));
  const [errors, setErrors] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [flash, setFlash] = useState<string | null>(null);
  const uid = useId();
  const locked = (record?.attempts ?? 0) > 0;
  const s = d.settings;
  const setS = (patch: Partial<QuizSettings>) => setD((x) => ({ ...x, settings: { ...x.settings, ...patch } }));
  const setQ = (qs: DraftQuestion[]) => setD((x) => ({ ...x, questions: qs }));
  const studyKind = d.kind !== "practice";
  const afterDays = s.after?.days ?? 0;

  const live = useMemo(() => d, [d]);

  const save = async () => {
    const errs = validateDraft(d);
    setErrors(errs);
    setFlash(null);
    if (errs.length) return;
    setBusy(true);
    try {
      const body = JSON.stringify(toPayload(d));
      if (record) {
        await quizApi(`/api/admin/quizzes/${record.id}`, { method: "PUT", body });
        setFlash("Saved");
        onSaved(record.id);
      } else {
        const res = await quizApi<{ id: string }>("/api/admin/quizzes", { method: "POST", body });
        onSaved(res.id);
      }
    } catch (e) {
      setErrors([e instanceof Error ? e.message : "Save failed"]);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="aq-editor">
      <div className="aq-form">
        {locked && (
          <p className="aq-banner">
            This quiz has {record!.attempts} attempts, so its questions are locked to keep results comparable. You can still change
            the schedule and settings, or duplicate the quiz to change questions.
          </p>
        )}
        <section aria-labelledby={`${uid}-basics`}>
          <h2 id={`${uid}-basics`}>Basics</h2>
          <Field id={`${uid}-t`} label="Title">
            <input id={`${uid}-t`} value={d.title} onChange={(e) => setD({ ...d, title: e.target.value })} />
          </Field>
          <Field id={`${uid}-d`} label="Description (shown to learners)">
            <textarea id={`${uid}-d`} rows={2} value={d.description} onChange={(e) => setD({ ...d, description: e.target.value })} />
          </Field>
          <div className="aq-grid2">
            <Field id={`${uid}-k`} label="Kind">
              <select id={`${uid}-k`} value={d.kind} onChange={(e) => {
                const kind = e.target.value as QuizKind;
                setD({ ...d, kind, settings: { ...d.settings, feedback: kind === "practice" ? d.settings.feedback : "never" } });
              }}>
                {(Object.keys(KIND_LABEL) as QuizKind[]).map((k) => <option key={k} value={k}>{KIND_LABEL[k]}</option>)}
              </select>
            </Field>
            <Field id={`${uid}-f`} label="Show results" hint={studyKind ? "Study tests default to never: showing answers is itself a learning event and could cue later recall." : undefined}>
              <select id={`${uid}-f`} value={s.feedback} onChange={(e) => setS({ feedback: e.target.value as FeedbackPolicy })}>
                {(Object.keys(FEEDBACK_LABEL) as FeedbackPolicy[]).map((f) => <option key={f} value={f}>{FEEDBACK_LABEL[f]}</option>)}
              </select>
            </Field>
          </div>
        </section>

        <section aria-labelledby={`${uid}-sched`}>
          <h2 id={`${uid}-sched`}>Schedule and rules</h2>
          <div className="aq-grid2">
            <Field id={`${uid}-o`} label="Opens (optional)">
              <input id={`${uid}-o`} type="datetime-local" value={toLocalInput(s.open_at)} onChange={(e) => setS({ open_at: fromLocalInput(e.target.value) })} />
            </Field>
            <Field id={`${uid}-c`} label="Closes (optional)">
              <input id={`${uid}-c`} type="datetime-local" value={toLocalInput(s.close_at)} onChange={(e) => setS({ close_at: fromLocalInput(e.target.value) })} />
            </Field>
          </div>
          <div className="aq-grid2">
            <Field id={`${uid}-rel`} label="Relative availability">
              <select id={`${uid}-rel`} value={s.after?.anchor ?? ""} onChange={(e) => {
                const v = e.target.value;
                setS({ after: v ? { anchor: v as "first_reading", days: s.after?.days ?? 7, window_days: s.after?.window_days } : null });
              }}>
                <option value="">None</option>
                <option value="first_reading">After the learner&apos;s first reading session</option>
                <option value="document">After the learner opens a document</option>
                <option value="quiz_submitted">After the learner submits another quiz</option>
              </select>
            </Field>
            {s.after && (
              <div className="aq-grid2">
                <Field id={`${uid}-days`} label="Days after">
                  <input id={`${uid}-days`} type="number" min={0} value={afterDays} onChange={(e) => setS({ after: { ...s.after!, days: Number(e.target.value) } })} />
                </Field>
                <Field id={`${uid}-win`} label="Open for (days)">
                  <input id={`${uid}-win`} type="number" min={0} value={s.after.window_days ?? ""} placeholder="no limit" onChange={(e) => setS({ after: { ...s.after!, window_days: e.target.value ? Number(e.target.value) : undefined } })} />
                </Field>
              </div>
            )}
          </div>
          {s.after?.anchor === "document" && (
            <Field id={`${uid}-doc`} label="Document title or content hash">
              <input id={`${uid}-doc`} value={s.after.document ?? ""} onChange={(e) => setS({ after: { ...s.after!, document: e.target.value } })} />
            </Field>
          )}
          {s.after?.anchor === "quiz_submitted" && (
            <Field id={`${uid}-aq`} label="Earlier quiz id">
              <input id={`${uid}-aq`} value={s.after.quiz_id ?? ""} onChange={(e) => setS({ after: { ...s.after!, quiz_id: e.target.value } })} />
            </Field>
          )}
          <div className="aq-grid3">
            <Field id={`${uid}-tl`} label="Time limit (minutes)">
              <input id={`${uid}-tl`} type="number" min={0} value={s.time_limit_seconds ? s.time_limit_seconds / 60 : ""} placeholder="none"
                onChange={(e) => setS({ time_limit_seconds: e.target.value ? Math.round(Number(e.target.value) * 60) : undefined })} />
            </Field>
            <Field id={`${uid}-ma`} label="Attempts allowed" hint="0 = unlimited">
              <input id={`${uid}-ma`} type="number" min={0} value={s.max_attempts} onChange={(e) => setS({ max_attempts: Number(e.target.value) })} />
            </Field>
            <Field id={`${uid}-au`} label="Audience">
              <select id={`${uid}-au`} value={s.audience.type} onChange={(e) => setS({ audience: { type: e.target.value as "all", value: e.target.value === "all" ? undefined : s.audience.value } })}>
                <option value="all">All users</option>
                <option value="cohort">A cohort</option>
                <option value="group">A group label</option>
              </select>
            </Field>
          </div>
          {s.audience.type !== "all" && (
            <Field id={`${uid}-av`} label={s.audience.type === "cohort" ? "Cohort name" : "Group label"}>
              <input id={`${uid}-av`} value={s.audience.value ?? ""} onChange={(e) => setS({ audience: { ...s.audience, value: e.target.value } })} />
            </Field>
          )}
          <div className="aq-checks">
            <label><input type="checkbox" checked={s.shuffle_questions} onChange={(e) => setS({ shuffle_questions: e.target.checked })} /> Shuffle question order</label>
            <label><input type="checkbox" checked={s.shuffle_options} onChange={(e) => setS({ shuffle_options: e.target.checked })} /> Shuffle answer options</label>
            <label><input type="checkbox" checked={s.no_going_back} onChange={(e) => setS({ no_going_back: e.target.checked })} /> One question at a time, no going back (recall tests)</label>
          </div>
        </section>

        <section aria-labelledby={`${uid}-qs`}>
          <h2 id={`${uid}-qs`}>Questions</h2>
          <ol className="aq-qs">
            {d.questions.map((q, i) => (
              <QuestionCard
                key={i}
                q={q} i={i} count={d.questions.length} locked={locked}
                onChange={(nq) => setQ(d.questions.map((x, j) => (j === i ? nq : x)))}
                onMove={(delta) => setQ(moveQuestion(d.questions, i, delta))}
                onDuplicate={() => setQ(duplicateQuestion(d.questions, i))}
                onRemove={() => setQ(d.questions.filter((_, j) => j !== i))}
              />
            ))}
          </ol>
          <Button variant="secondary" disabled={locked} onClick={() => setQ([...d.questions, newQuestion("single_choice")])}>+ Add question</Button>
        </section>

        {errors.length > 0 && (
          <div role="alert" className="quiz-error">
            <ul>{errors.map((e) => <li key={e}>{e}</li>)}</ul>
          </div>
        )}
        <div className="aq-row aq-sticky">
          <Button variant="primary" disabled={busy} onClick={save}>{record ? "Save changes" : "Save draft"}</Button>
          {flash && <span className="aq-muted" aria-live="polite">{flash}</span>}
        </div>
      </div>
      <Preview d={live} />
    </div>
  );
}
