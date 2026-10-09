"use client";

import { useRouter } from "next/navigation";
import { AddContentButton } from "./AddContentButton";
import { DriveImportFooter } from "./DriveImportFooter";

/** Compose existing import flows; format support remains owned by each importer. */
export function LibraryImports() {
  const router = useRouter();
  return <section id="library-imports" aria-labelledby="library-imports-title" tabIndex={-1} className="ui-card" style={{ padding: 18, marginBottom: 20 }}>
    <h2 id="library-imports-title" style={{ fontSize: 16, marginBottom: 8 }}>Add a reading</h2>
    <p style={{ marginBottom: 12 }}>Choose a file, web page, or pasted text with Add content, or pick a PDF from Google Drive.</p>
    <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 16 }}>
      <AddContentButton />
      <DriveImportFooter onImported={() => router.refresh()} />
    </div>
  </section>;
}
