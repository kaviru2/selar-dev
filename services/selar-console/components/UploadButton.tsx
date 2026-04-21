"use client";

import { useState } from "react";
import { Icon } from "@/components/ui/Icon";
import { useRouter } from "next/navigation";

export function UploadButton() {
  const router = useRouter();
  const [uploading, setUploading] = useState(false);

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setUploading(true);
    const formData = new FormData();
    formData.append("file", file);

    try {
      const res = await fetch("/api/documents/upload", {
        method: "POST",
        body: formData,
      });
      if (res.ok) {
        router.refresh();
      } else {
        const d = await res.json();
        alert(d.error || "Upload failed");
      }
    } catch (err) {
      console.error(err);
      alert("Network error");
    } finally {
      setUploading(false);
    }
  };

  return (
    <label className="btn primary" style={{ cursor: uploading ? "wait" : "pointer", display: "inline-flex", alignItems: "center", gap: 6, opacity: uploading ? 0.7 : 1 }}>
      <Icon name="upload" size={12} /> {uploading ? "Uploading..." : "Upload PDF"}
      <input type="file" accept="application/pdf" style={{ display: "none" }} onChange={handleUpload} disabled={uploading} />
    </label>
  );
}
