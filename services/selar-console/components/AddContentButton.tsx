"use client";

import { DragEvent, FormEvent, useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Icon } from "@/components/ui/Icon";
import { ProcessingDisclosure } from "@/components/ProcessingDisclosure";
import { uploadPDF, fileContentType } from "@/lib/upload";

type AddMode = "web" | "text" | "pdf";
type UploadProgress = { completed: number; total: number } | null;

const MAX_PDF_SIZE = 50 * 1024 * 1024;
const MAX_PDF_BATCH = 10;

export function AddContentButton() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const dialogRef = useRef<HTMLDialogElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [mode, setMode] = useState<AddMode>("web");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [uploadProgress, setUploadProgress] = useState<UploadProgress>(null);
  const [dragActive, setDragActive] = useState(false);
  const incomingUrl = searchParams.get("addUrl") || "";

  useEffect(() => {
    if (incomingUrl) dialogRef.current?.showModal();
  }, [incomingUrl]);

  function open() {
    setError("");
    dialogRef.current?.showModal();
  }

  function chooseMode(value: AddMode) {
    setMode(value);
    setError("");
  }

  function acceptFiles(files: File[]) {
    if (!files.length) return;
    const nonPdfs = files.filter((file) => !fileContentType(file.name));
    const oversized = files.filter((file) => file.size > (fileContentType(file.name) === 'application/pdf' ? MAX_PDF_SIZE : 10 * 1024 * 1024));
    if (nonPdfs.length || oversized.length) {
      const problems = [
        nonPdfs.length ? `${nonPdfs.map((file) => file.name).join(", ")}: supported formats are PDF, Markdown, TXT and DOCX (.doc/.docm unsupported)` : "",
        oversized.length ? `${oversized.map((file) => file.name).join(", ")}: limit is 50 MB for PDF, 10 MB for other files` : "",
      ].filter(Boolean);
      setError(problems.join(". "));
      return;
    }

    setSelectedFiles((current) => {
      const unique = [...current];
      for (const file of files) {
        const duplicate = unique.some((item) => item.name === file.name && item.size === file.size && item.lastModified === file.lastModified);
        if (!duplicate) unique.push(file);
      }
      if (unique.length > MAX_PDF_BATCH) {
        setError(`Upload up to ${MAX_PDF_BATCH} files at a time`);
        return unique.slice(0, MAX_PDF_BATCH);
      }
      setError("");
      return unique;
    });
    if (fileInputRef.current) fileInputRef.current.value = "";
  }

  function dropFile(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragActive(false);
    acceptFiles(Array.from(event.dataTransfer.files));
  }

  function formatFileSize(bytes: number) {
    return bytes >= 1024 * 1024
      ? `${(bytes / (1024 * 1024)).toFixed(1)} MB`
      : `${Math.max(1, Math.round(bytes / 1024))} KB`;
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    const form = new FormData(event.currentTarget);
    let uploadedfiles = 0;
    try {
      if (mode === "pdf") {
        if (!selectedFiles.length) throw new Error("Choose at least one PDF first");
        for (let index = 0; index < selectedFiles.length; index += 1) {
          const file = selectedFiles[index];
          setUploadProgress({ completed: index, total: selectedFiles.length });
          try {
            await uploadPDF(file);
          } catch (cause) {
            throw new Error(`${file.name}: ${cause instanceof Error ? cause.message : "Unable to upload PDF"}`);
          }
          uploadedfiles = index + 1;
        }
        setUploadProgress({ completed: selectedFiles.length, total: selectedFiles.length });
      } else {
        const response = await fetch("/api/documents/add", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            source_type: mode,
            url: mode === "web" ? form.get("url") : undefined,
            title: form.get("title"),
            text: mode === "text" ? form.get("text") : undefined,
          }),
        });
        const payload = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(payload.error || "Unable to add content");
      }
      setSelectedFiles([]);
      dialogRef.current?.close();
      router.refresh();
    } catch (cause) {
      if (mode === "pdf" && uploadedfiles > 0) setSelectedFiles(selectedFiles.slice(uploadedfiles));
      const message = cause instanceof Error ? cause.message : "Unable to add content";
      setError(uploadedfiles > 0 ? `${uploadedfiles} uploaded successfully. ${message}` : message);
    } finally {
      setUploadProgress(null);
      setSubmitting(false);
    }
  }

  return (
    <>
      <button className="btn primary" onClick={open}>
        <Icon name="plus" size={12} /> Add content
      </button>
      <dialog ref={dialogRef} className="add-content-dialog" onClose={() => setError("")}>
        <form onSubmit={submit}>
          <div className="add-content-head">
            <div>
              <span className="add-content-eyebrow">NEW SOURCE</span>
              <h2>Add to your research library</h2>
              <p>Bring in a link, your own notes, or a research paper.</p>
            </div>
            <button type="button" className="dialog-close" aria-label="Close" onClick={() => dialogRef.current?.close()}><Icon name="x" size={16} /></button>
          </div>
          <div className="add-content-tabs" role="tablist">
            {(["web", "text", "pdf"] as AddMode[]).map((value) => {
              const details = value === "web"
                ? { icon: "link" as const, label: "From the web", hint: "Article or blog" }
                : value === "text"
                  ? { icon: "note" as const, label: "Paste text", hint: "Notes or Markdown" }
                  : { icon: "doc" as const, label: "Upload PDF & files", hint: "PDF, Markdown, TXT, DOCX" };
              return <button key={value} type="button" role="tab" aria-selected={mode === value} className={mode === value ? "active" : ""} onClick={() => chooseMode(value)}>
                <span className="source-tab-icon"><Icon name={details.icon} size={17} /></span>
                <span><strong>{details.label}</strong><small>{details.hint}</small></span>
              </button>
            })}
          </div>
          <ProcessingDisclosure />
          {mode === "web" && (
            <div className="add-source-fields">
              <label>
                <span>Article URL</span>
                <div className="url-field"><Icon name="link" size={15} /><input name="url" type="url" required placeholder="https://example.org/article" defaultValue={incomingUrl} autoFocus /></div>
                <small>Works with public articles, blogs, and documentation pages.</small>
              </label>
              <label>
                <span>Title <small>Optional — we normally detect it</small></span>
                <input name="title" placeholder="Leave blank to use the article title" />
              </label>
            </div>
          )}
          {mode === "text" && (
            <div className="add-source-fields">
              <label>
                <span>Title</span>
                <input name="title" placeholder="e.g. Notes on visual retrieval" autoFocus />
              </label>
              <label>
                <span>Text or Markdown</span>
                <textarea name="text" required rows={10} placeholder="Paste an article, newsletter, or your research notes…" />
              </label>
            </div>
          )}
          {mode === "pdf" && (
            <div className="pdf-upload-section">
              <input ref={fileInputRef} id="pdf-upload-input" type="file" accept=".pdf,.md,.markdown,.txt,.docx" multiple aria-label="Choose PDF, Markdown, TXT or DOCX files to upload" onChange={(event) => acceptFiles(Array.from(event.target.files || []))} />
              <div
                className={`pdf-dropzone${dragActive ? " drag-active" : ""}${selectedFiles.length ? " compact" : ""}`}
                onDragEnter={(event) => { event.preventDefault(); setDragActive(true); }}
                onDragOver={(event) => event.preventDefault()}
                onDragLeave={(event) => { if (event.currentTarget === event.target) setDragActive(false); }}
                onDrop={dropFile}
                onClick={() => fileInputRef.current?.click()}
                role="button"
                aria-label="Drop PDF files or choose PDF files to upload"
                tabIndex={0}
                onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); fileInputRef.current?.click(); } }}
              >
                {selectedFiles.length ? (
                  <>
                    <span className="pdf-upload-icon"><Icon name="plus" size={18} /></span>
                    <div className="pdf-drop-copy"><strong>Add more files</strong><span>Drop them here or <u>choose files</u></span></div>
                    <small>Up to {MAX_PDF_BATCH} per batch</small>
                  </>
                ) : (
                  <>
                    <span className="pdf-upload-icon"><Icon name="upload" size={25} /></span>
                    <div className="pdf-drop-copy"><strong>Drop your files here</strong><span>or <u>choose files</u> from your computer</span></div>
                    <small>PDF up to 50 MB · Markdown, TXT, DOCX up to 10 MB · {MAX_PDF_BATCH} per batch</small>
                  </>
                )}
              </div>
              {selectedFiles.length > 0 && (
                <div className="pdf-queue">
                  <div className="pdf-queue-head">
                    <strong>{selectedFiles.length} {selectedFiles.length === 1 ? "PDF" : "files"} ready</strong>
                    <span>{formatFileSize(selectedFiles.reduce((total, file) => total + file.size, 0))} total</span>
                  </div>
                  <div className="pdf-queue-list">
                    {selectedFiles.map((file, index) => (
                      <div className="pdf-file-row" key={`${file.name}-${file.size}-${file.lastModified}`}>
                        <span className="pdf-file-icon"><Icon name="doc" size={18} /></span>
                        <div className="pdf-file-copy"><strong>{file.name}</strong><span>{formatFileSize(file.size)} · {uploadProgress && index < uploadProgress.completed ? "Uploaded" : uploadProgress?.completed === index ? "Uploading" : "Ready"}</span></div>
                        <button type="button" className="remove-file" disabled={submitting} aria-label={`Remove ${file.name}`} onClick={() => setSelectedFiles((files) => files.filter((_, fileIndex) => fileIndex !== index))}><Icon name="x" size={14} /></button>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              <div role="status" aria-live="polite">
                {uploadProgress ? `Uploading PDF ${Math.min(uploadProgress.completed + 1, uploadProgress.total)} of ${uploadProgress.total}` : selectedFiles.length ? `${selectedFiles.length} files ready to upload` : ""}
              </div>
              <div className="pdf-feature-row">
                <span><Icon name="check" size={13} /> Text and citations</span>
                <span>DOCX: text and tables only. Tracked changes, comments and footnotes are rejected; .doc/.docm unsupported. Export a reviewed PDF for full visual fidelity.</span>
                <span><Icon name="check" size={13} /> Semantic links</span>
              </div>
            </div>
          )}
          {error && <div className="form-error" role="alert">{error}</div>}
          <div className="add-content-actions">
            <div className="add-content-assurance">Prototype testing only</div>
            <button type="button" className="btn" onClick={() => dialogRef.current?.close()}>Cancel</button>
            <button type="submit" className="btn primary add-submit" disabled={submitting || (mode === "pdf" && !selectedFiles.length)}>
              {submitting ? <><Icon name="spinner" size={12} className="animate-spin" /> {mode === "pdf" && uploadProgress ? `Uploading ${Math.min(uploadProgress.completed + 1, uploadProgress.total)} of ${uploadProgress.total}` : "Importing…"}</> : <>{mode === "web" ? "Import article" : mode === "text" ? "Save text" : selectedFiles.length ? `Upload ${selectedFiles.length} file${selectedFiles.length === 1 ? "" : "s"}` : "Upload files"} <Icon name="arrow_right" size={12} /></>}
            </button>
          </div>
        </form>
      </dialog>
    </>
  );
}
