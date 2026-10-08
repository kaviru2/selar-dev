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

const PDF_TYPE = "application/pdf";

async function errorFrom(response: Response, fallback: string): Promise<Error> {
  const payload = await response.json().catch(() => ({}));
  return new Error((payload && typeof payload.error === "string" && payload.error) || fallback);
}

export async function uploadPDF(file: File): Promise<void> {
  if (!file.name.toLowerCase().endsWith(".pdf")) throw new Error("Only PDF files can be uploaded");

  const planResponse = await fetch("/api/documents/upload-url", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ filename: file.name, size: file.size, content_type: PDF_TYPE }),
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
