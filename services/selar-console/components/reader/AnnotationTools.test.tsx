import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AnnotationEditor, ColorSwatches, HighlightsList, annotationColorFor, isSubmitChord } from "./AnnotationTools";
import type { Annotation } from "@/lib/api";

let host: HTMLDivElement;
let root: Root;

beforeEach(() => {
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  window.sessionStorage.clear();
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

const buttonNamed = (name: string, scope: ParentNode = host) =>
  Array.from(scope.querySelectorAll("button")).find((b) => b.textContent === name || b.getAttribute("aria-label") === name)!;
const click = async (name: string, scope?: ParentNode) => act(async () => { buttonNamed(name, scope).click(); });
const typeInto = async (field: HTMLTextAreaElement, value: string) => act(async () => {
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(field, value);
  field.dispatchEvent(new Event("input", { bubbles: true }));
});
const key = async (target: Element, init: KeyboardEventInit) => act(async () => { target.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, ...init })); });

const mark = {
  user_id: "owner", chunk_id: null, created_at: "", updated_at: "", id: "a", document_id: "d", page: 2, color: "yellow",
  type: "highlight", comment: "note", bbox: [],
  anchor: { version: 1, source_hash: "s", exact: "Exact quote", prefix: "", suffix: "", start: 0, end: 11 },
} as Annotation;

describe("AnnotationEditor", () => {
  it("focuses a labeled non-prompt editor and keeps failed saves editable", async () => {
    const save = vi.fn().mockRejectedValue(new Error("Offline"));
    const cancel = vi.fn();
    await act(async () => root.render(<AnnotationEditor title="Edit highlight" quote="Exact quote" color="yellow" comment="note" onSave={save} onCancel={cancel} />));
    expect(host.querySelector('textarea[aria-label="Note"]')).toBe(document.activeElement);
    await click("Save");
    expect(save).toHaveBeenCalledWith("yellow", "note");
    expect(host.querySelector('[role="alert"]')?.textContent).toBe("Offline");
    await click("Cancel");
    expect(cancel).toHaveBeenCalledOnce();
  });

  it("saves with Cmd/Ctrl+Enter and picks colours from one-click swatches", async () => {
    const save = vi.fn().mockResolvedValue(undefined);
    await act(async () => root.render(<AnnotationEditor title="Add note" quote="q" color="yellow" comment="" onSave={save} onCancel={vi.fn()} />));
    await click("Green");
    expect(host.querySelector('[role="radio"][aria-label="Green"]')?.getAttribute("aria-checked")).toBe("true");
    const field = host.querySelector("textarea")!;
    await typeInto(field, "  my words  ");
    await key(field, { key: "Enter", metaKey: true });
    expect(save).toHaveBeenCalledWith("sage", "my words");
  });

  it("Escape cancels", async () => {
    const cancel = vi.fn();
    await act(async () => root.render(<AnnotationEditor title="Add note" color="yellow" comment="" onSave={vi.fn()} onCancel={cancel} />));
    await key(host.querySelector("textarea")!, { key: "Escape" });
    expect(cancel).toHaveBeenCalledOnce();
  });

  it("keeps an unsaved draft across an accidental close and clears it after save", async () => {
    await act(async () => root.render(<AnnotationEditor key="1" title="Add note" color="yellow" comment="" draftKey="doc:new:1:0" onSave={vi.fn()} onCancel={vi.fn()} />));
    await typeInto(host.querySelector("textarea")!, "half-written thought");
    await act(async () => root.render(<div />));
    const save = vi.fn().mockResolvedValue(undefined);
    await act(async () => root.render(<AnnotationEditor key="2" title="Add note" color="yellow" comment="" draftKey="doc:new:1:0" onSave={save} onCancel={vi.fn()} />));
    expect(host.querySelector("textarea")!.value).toBe("half-written thought");
    expect(host.textContent).toContain("Restored your unsaved note");
    await click("Save");
    expect(save).toHaveBeenCalledWith("yellow", "half-written thought");
    expect(window.sessionStorage.getItem("selar.noteDraft.doc:new:1:0")).toBeNull();
  });

  it("offers delete with confirmation when editing a saved mark", async () => {
    const remove = vi.fn().mockResolvedValue(undefined);
    await act(async () => root.render(<AnnotationEditor title="Edit highlight" color="yellow" comment="" onSave={vi.fn()} onCancel={vi.fn()} onDelete={remove} />));
    await click("Delete highlight");
    expect(remove).not.toHaveBeenCalled();
    await click("Confirm delete");
    expect(remove).toHaveBeenCalledOnce();
  });
});

describe("colour helpers", () => {
  it("maps Settings colours onto stored annotation colours", () => {
    expect(annotationColorFor("green")).toBe("sage");
    expect(annotationColorFor("pink")).toBe("coral");
    expect(annotationColorFor("yellow")).toBe("yellow");
    expect(annotationColorFor("blue")).toBe("yellow");
    expect(annotationColorFor(undefined)).toBe("yellow");
  });

  it("recognises the submit chord", () => {
    expect(isSubmitChord({ key: "Enter", metaKey: true, ctrlKey: false })).toBe(true);
    expect(isSubmitChord({ key: "Enter", metaKey: false, ctrlKey: true })).toBe(true);
    expect(isSubmitChord({ key: "Enter", metaKey: false, ctrlKey: false })).toBe(false);
  });

  it("renders swatches as a radio group with human names and arrow-key movement", async () => {
    const change = vi.fn();
    await act(async () => root.render(<ColorSwatches value="yellow" onChange={change} />));
    const radios = Array.from(host.querySelectorAll('[role="radio"]'));
    expect(radios.map((r) => r.getAttribute("aria-label"))).toEqual(["Yellow", "Orange", "Pink", "Green"]);
    await key(radios[0], { key: "ArrowRight" });
    expect(change).toHaveBeenCalledWith("wheat");
  });
});

describe("HighlightsList", () => {
  it("lists real quotes, labels legacy marks, jumps, edits and confirms deletion", async () => {
    const jump = vi.fn(), edit = vi.fn(), remove = vi.fn().mockResolvedValue(undefined);
    await act(async () => root.render(<HighlightsList annotations={[mark, { ...mark, id: "legacy", anchor: null }]} sourceHash="s" onJump={jump} onEdit={edit} onDelete={remove} />));
    expect(host.textContent).toContain("Exact quote");
    expect(host.textContent).toContain("Legacy rectangle — no saved quote");
    const row = host.querySelector('[data-annotation-id="a"]')!;
    await click("Jump to page 2", row);
    expect(jump).toHaveBeenCalledWith(mark);
    await click("Edit", row);
    expect(edit).toHaveBeenCalledWith(mark);
    await click("Delete", row);
    expect(remove).not.toHaveBeenCalled();
    await click("Confirm delete", row);
    expect(remove).toHaveBeenCalledWith(mark);
  });

  it("orders by page then position, shows a helpful empty state and closes on Escape", async () => {
    const close = vi.fn();
    const later = { ...mark, id: "later", anchor: { ...mark.anchor!, exact: "Second", start: 50, end: 56 } };
    const first = { ...mark, id: "first", anchor: { ...mark.anchor!, exact: "First", start: 2, end: 7 }, created_at: "2026-10-10" };
    await act(async () => root.render(<HighlightsList annotations={[later, first]} sourceHash="s" onJump={vi.fn()} onEdit={vi.fn()} onDelete={vi.fn()} onClose={close} />));
    expect(Array.from(host.querySelectorAll("li")).map((li) => li.getAttribute("data-annotation-id"))).toEqual(["first", "later"]);
    await key(host.querySelector("li button")!, { key: "Escape" });
    expect(close).toHaveBeenCalledOnce();
    await act(async () => root.render(<HighlightsList annotations={[]} sourceHash="s" onJump={vi.fn()} onEdit={vi.fn()} onDelete={vi.fn()} />));
    expect(host.textContent).toContain("No highlights yet");
  });

  it("never enables jump/edit/delete when the source changed", async () => {
    await act(async () => root.render(<HighlightsList annotations={[mark]} sourceHash="changed" onJump={vi.fn()} onEdit={vi.fn()} onDelete={vi.fn()} />));
    expect(host.textContent).toContain("Source changed");
    const actions = Array.from(host.querySelectorAll("button")).filter((b) => /Jump|Edit|Delete/.test(`${b.textContent} ${b.getAttribute("aria-label") ?? ""}`));
    expect(actions.length).toBeGreaterThanOrEqual(3);
    expect(actions.every((b) => b.disabled)).toBe(true);
  });
});
