// components/admin/GroupLabelEditor.tsx — inline editor for the neutral
// admin-only group label (PATCH /api/admin/users/:id).

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

export function GroupLabelEditor({ userId, initial }: { userId: string; initial: string }) {
  const router = useRouter();
  const [value, setValue] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const [status, setStatus] = useState<"" | "saving" | "error">("");
  const [error, setError] = useState("");

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setStatus("saving");
    setError("");
    try {
      const res = await fetch(`/api/admin/users/${userId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group_label: value.trim() }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
      setSaved(body.group_label ?? value.trim());
      setValue(body.group_label ?? value.trim());
      setStatus("");
      router.refresh();
    } catch (err) {
      setStatus("error");
      setError(err instanceof Error ? err.message : "Could not save");
    }
  };

  return (
    <form onSubmit={save} className="adm-group-form">
      <input
        aria-label="Group label"
        value={value}
        maxLength={64}
        placeholder="none"
        onChange={(e) => setValue(e.target.value)}
      />
      <button type="submit" className="ui-btn ui-btn--sm" disabled={status === "saving" || value.trim() === saved}>
        {status === "saving" ? "Saving…" : "Save"}
      </button>
      {status === "error" && <small role="alert" className="adm-error">{error}</small>}
      <small className="adm-muted">Only admins can see this. It does not change what the user sees in the app.</small>
    </form>
  );
}
