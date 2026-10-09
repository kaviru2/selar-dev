// Client-side PDF upload. The API decides the path:
//   - "direct": PUT the file to a short-lived presigned object-storage URL,
//     then ask the API to verify and register it (avoids the ~4.5 MB Vercel
//     Functions request-body limit for large PDFs);
//   - "multipart": post the file through the API (local storage / Docker).

type UploadPlan =
  | { mode: "multipart"; max_bytes?: number }
  | {
      mode: "direct";
      upload_id: string;
      upload: { method: string; url: string; headers: Record<string, string> };
      max_bytes?: number;
    };

export function fileContentType(name: string): string | undefined {
  const extension = name.toLowerCase().split('.').pop() || '';
  return ({pdf: 'application/pdf', md: 'text/markdown', markdown: 'text/markdown', txt: 'text/plain', docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'} as Record<string,string>)[extension];
}

async function errorFrom(response: Response, fallback: string): Promise<Error> {
  const payload = await response.json().catch(() => ({}));
  return new Error((payload && typeof payload.error === "string" && payload.error) || fallback);
}

export async function uploadPDF(file: File): Promise<void> {
  const mime = fileContentType(file.name);
  if (!mime) throw new Error("Only PDF, Markdown, TXT and DOCX files can be uploaded");
  const limit = mime === 'application/pdf' ? 50 * 1024 * 1024 : 10 * 1024 * 1024;
  if (file.size > limit || !file.size) throw new Error('File is empty or exceeds its size limit');

  const planResponse = await fetch("/api/documents/upload-url", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ filename: file.name, size: file.size, content_type: mime }),
  });
  if (!planResponse.ok) throw await errorFrom(planResponse, "Unable to prepare upload");
  const plan = (await planResponse.json()) as UploadPlan;

  if (plan.mode !== "direct") {
    const form = new FormData();
    form.append("file", file);
    const response = await fetch("/api/documents/upload", { method: "POST", body: form });
    if (!response.ok) throw await errorFrom(response, "Unable to upload PDF");
    return;
  }

  // Never send console cookies to the storage origin; the URL is the credential.
  const stored = await fetch(plan.upload.url, {
    method: plan.upload.method || "PUT",
    headers: plan.upload.headers,
    body: file,
    credentials: "omit",
  });
  if (!stored.ok) throw new Error(`Storage rejected the upload (HTTP ${stored.status})`);

  const complete = await fetch("/api/documents/upload-complete", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ upload_id: plan.upload_id, filename: file.name }),
  });
  if (!complete.ok) throw await errorFrom(complete, "Unable to register uploaded PDF");
}
