"use client";

// ImportPanel — the "easy deploy" path: drop or paste a quiz file
// (YAML, JSON or Markdown), check it, then import as draft or publish.

import { useRef, useState } from "react";
import { Button } from "@/components/ui/Button";
import { quizApi } from "@/lib/quiz/client";
import { KIND_LABEL, type QuizKind } from "@/lib/quiz/quiz";

interface Preview {
  title: string;
  kind: QuizKind;
  settings: { feedback: string; max_attempts: number; audience: { type: string; value?: string }; time_limit_seconds?: number; no_going_back?: boolean };
  questions: { type: string; prompt: string }[];
}

export function guessFormat(name: string, text: string): { filename: string; type: string } {
  const lower = name.toLowerCase();
  if (lower.endsWith(".md") || lower.endsWith(".markdown")) return { filename: name, type: "text/markdown" };
  if (lower.endsWith(".json")) return { filename: name, type: "application/json" };
  if (lower.endsWith(".yaml") || lower.endsWith(".yml")) return { filename: name, type: "application/yaml" };
  const t = text.trimStart();
  if (t.startsWith("{")) return { filename: "quiz.json", type: "application/json" };
  if (/^#\s|^---[\s\S]*?---\s*#/m.test(t) && /^##\s/m.test(t)) return { filename: "quiz.md", type: "text/markdown" };
  return { filename: "quiz.yaml", type: "application/yaml" };
}

export function ImportPanel({ onImported }: { onImported: (id: string) => void }) {
  const [text, setText] = useState("");
  const [name, setName] = useState("");
  const [preview, setPreview] = useState<Preview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const send = async (query: string) => {
    const f = guessFormat(name, text);
    return quizApi<{ id?: string; draft: Preview }>(`/api/admin/quizzes/import?filename=${encodeURIComponent(f.filename)}${query}`, {
      method: "POST",
      headers: { "Content-Type": f.type },
      body: text,
    });
  };

  const check = async () => {
    setBusy(true); setError(null); setPreview(null);
    try {
      setPreview((await send("&dry_run=1")).draft);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not read the file");
    } finally { setBusy(false); }
  };

  const doImport = async (publish: boolean) => {
    setBusy(true); setError(null);
    try {
      const res = await send(publish ? "&publish=1" : "");
      if (res.id) onImported(res.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Import failed");
    } finally { setBusy(false); }
  };

  const loadFile = async (f: File | undefined) => {
    if (!f) return;
    setName(f.name);
    setText(await f.text());
    setPreview(null); setError(null);
  };

  return (
    <section className="aq-import" aria-labelledby="aq-import-h">
      <h2 id="aq-import-h">Import a quiz file</h2>
      <p className="aq-muted">
        YAML, JSON or Markdown. Templates: <a href="/quiz-templates/quiz-template.md" download>Markdown</a> · <a href="/quiz-templates/quiz-template.yaml" download>YAML</a> · <a href="/quiz-templates/quiz-template.json" download>JSON</a>. Format guide in <code>docs/QUIZZES.md</code>.
      </p>
      <div
        className="aq-drop"
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => { e.preventDefault(); void loadFile(e.dataTransfer.files?.[0]); }}
      >
        <Button variant="secondary" size="sm" onClick={() => fileRef.current?.click()}>Choose file…</Button>
        <input ref={fileRef} type="file" accept=".md,.markdown,.yaml,.yml,.json,text/*,application/json" hidden onChange={(e) => loadFile(e.target.files?.[0])} />
        <span className="aq-muted">{name ? `Loaded ${name}` : "or drop it here, or paste below"}</span>
      </div>
      <label htmlFor="aq-import-text" className="sr-only">Quiz file contents</label>
      <textarea
        id="aq-import-text"
        className="aq-code"
        rows={12}
        spellCheck={false}
        value={text}
        placeholder={"# DEMO: My quiz\n\n## single_choice\nQuestion text\n- [x] Right\n- [ ] Wrong"}
        onChange={(e) => { setText(e.target.value); setPreview(null); }}
      />
      {error && <p role="alert" className="quiz-error">{error}</p>}
      <div className="aq-row">
        <Button variant="secondary" disabled={!text.trim() || busy} onClick={check}>Check file</Button>
        {preview && (
          <>
            <Button variant="secondary" disabled={busy} onClick={() => doImport(false)}>Import as draft</Button>
            <Button variant="primary" disabled={busy} onClick={() => doImport(true)}>Import and publish</Button>
          </>
        )}
      </div>
      {preview && (
        <div className="aq-preview-summary" aria-live="polite">
          <strong>{preview.title}</strong> · {KIND_LABEL[preview.kind] ?? preview.kind} · {preview.questions.length} question{preview.questions.length === 1 ? "" : "s"}
          <br />
          Results: {preview.settings.feedback.replace("_", " ")} · attempts: {preview.settings.max_attempts || "unlimited"} · audience: {preview.settings.audience.type}{preview.settings.audience.value ? ` (${preview.settings.audience.value})` : ""}
          {preview.settings.time_limit_seconds ? ` · ${Math.round(preview.settings.time_limit_seconds / 60)} min limit` : ""}
          {preview.settings.no_going_back ? " · no going back" : ""}
        </div>
      )}
    </section>
  );
}
