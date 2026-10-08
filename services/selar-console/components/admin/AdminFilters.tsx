// components/admin/AdminFilters.tsx — date range + group filter (GET form, so
// the server pages re-query with validated search params) and CSV export.

import Link from "next/link";
import { exportHref, type AdminFilters as Filters } from "@/lib/admin";

export function AdminFilters({ filters, groups, action }: { filters: Filters; groups: { label: string; users: number }[]; action: string }) {
  return (
    <form className="adm-filters" method="get" action={action}>
      <label>
        From <input type="date" name="from" defaultValue={filters.from} />
      </label>
      <label>
        To <input type="date" name="to" defaultValue={filters.to} />
      </label>
      <label>
        Group
        <select name="group" defaultValue={filters.group}>
          <option value="">All users</option>
          {groups.filter((g) => g.label).map((g) => (
            <option key={g.label} value={g.label}>
              {g.label} ({g.users})
            </option>
          ))}
        </select>
      </label>
      <button type="submit" className="ui-btn ui-btn--sm ui-btn--primary">Apply</button>
      <Link href={action} className="ui-btn ui-btn--sm ui-btn--ghost">Reset</Link>
      <span className="adm-muted adm-filters-note">Default: last 30 days (UTC).</span>
    </form>
  );
}

export function ExportLinks({ filters }: { filters: Filters }) {
  return (
    <details className="adm-export">
      <summary className="ui-btn ui-btn--sm">Export CSV</summary>
      <div className="adm-export-body">
        <p className="adm-muted">Exports use the current date and group filters. Exports never contain passwords. Email addresses are left out unless you choose otherwise.</p>
        <ul>
          <li><a href={exportHref("users", filters, false)} download>Per-user summary (no emails)</a></li>
          <li><a href={exportHref("events", filters, false)} download>Event log (no emails)</a></li>
        </ul>
        <p className="adm-muted">Only if you need to contact people, and the protocol allows it:</p>
        <ul>
          <li><a href={exportHref("users", filters, true)} download>Per-user summary including email addresses</a></li>
          <li><a href={exportHref("events", filters, true)} download>Event log including email addresses</a></li>
        </ul>
      </div>
    </details>
  );
}
