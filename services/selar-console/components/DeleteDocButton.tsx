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
      type="button"
      className="lib-delete"
      data-confirming={confirming}
      onClick={handleDelete}
      aria-label={confirming ? "Confirm: delete this reading" : "Delete reading"}
      title={confirming ? "Click again to confirm" : "Delete reading"}
    >
      <Icon name="trash" size={15} />
      {confirming && <span>Delete?</span>}
    </button>
  );
}
