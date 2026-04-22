// DeleteDocButton.tsx — Client component for deleting a document from the library.
"use client";

import { Icon } from "@/components/ui/Icon";
import { clientFetch } from "@/lib/api";
import { useRouter } from "next/navigation";
import { useState } from "react";

export function DeleteDocButton({ docId }: { docId: string }) {
  const router = useRouter();
  const [confirming, setConfirming] = useState(false);

  const handleDelete = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();

    if (!confirming) {
      setConfirming(true);
      setTimeout(() => setConfirming(false), 3000);
      return;
    }

    try {
      await clientFetch(`/api/documents/${docId}`, { method: "DELETE" });
      router.refresh();
    } catch (err) {
      console.error("Failed to delete document", err);
    }
  };

  return (
    <button
      onClick={handleDelete}
      title={confirming ? "Click again to confirm deletion" : "Delete document"}
      style={{
        width: 22, height: 22, borderRadius: 3, border: "none",
        background: confirming ? "rgba(192,68,58,0.08)" : "transparent",
        color: confirming ? "#c0443a" : "var(--ink-4)",
        cursor: "pointer", display: "grid", placeItems: "center",
        transition: "all 0.15s",
      }}
    >
      <Icon name="x" size={confirming ? 13 : 11} />
    </button>
  );
}
