// forbidden.tsx — rendered with a 403 when a page calls forbidden(), e.g. a
// signed-in non-admin opening /admin.
import Link from "next/link";

export default function Forbidden() {
  return (
    <main style={{ minHeight: "70vh", display: "grid", placeItems: "center", padding: 24 }}>
      <div className="ui-empty" style={{ maxWidth: 480 }}>
        <h2>Admins only</h2>
        <p>This area is for SELAR administrators. If you think you should have access, ask the project lead.</p>
        <div className="ui-empty-actions">
          <Link className="ui-btn ui-btn--primary" href="/library">Back to your library</Link>
        </div>
      </div>
    </main>
  );
}
