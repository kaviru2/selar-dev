"use client";

import { useState } from "react";
import { Icon } from "@/components/ui/Icon";
import { ProcessingDisclosure } from "@/components/ProcessingDisclosure";
import { useRouter } from "next/navigation";
import { uploadPDF } from "@/lib/upload";

export function UploadButton() {
  const router = useRouter();
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setError("");
    setStatus(`Uploading ${file.name}`);
    setUploading(true);
    try {
      await uploadPDF(file);
      setStatus(`${file.name} uploaded`);
      router.refresh();
    } catch (cause) {
      console.error(cause);
      setStatus("");
      setError(cause instanceof TypeError ? "Network error while uploading PDF" : cause instanceof Error ? cause.message : "Upload failed");
    } finally {
      e.target.value = "";
      setUploading(false);
    }
  };

  return (
    <div>
      <ProcessingDisclosure />
      <label className="btn primary" style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
        <Icon name="upload" size={12} /> {uploading ? "Uploading..." : "Upload PDF"}
        <input type="file" accept="application/pdf,.pdf" aria-label="Choose PDF to upload" onChange={handleUpload} disabled={uploading} />
      </label>
      <div role="status" aria-live="polite">{status}</div>
      {error && <div className="form-error" role="alert">{error}</div>}
    </div>
  );
}
