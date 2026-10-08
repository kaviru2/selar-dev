// admin/users/page.tsx — users table with per-user performance proxies.

import Link from "next/link";
import { AdminFilters, ExportLinks } from "@/components/admin/AdminFilters";
import { adminGet } from "@/lib/admin-server";
import { adminQuery, formatDay, formatDuration, parseAdminFilters, type AdminOverview, type AdminUserRow } from "@/lib/admin";

export const dynamic = "force-dynamic";

export default async function AdminUsersPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const filters = parseAdminFilters(await searchParams);
  const [users, { overview }] = await Promise.all([
    adminGet<AdminUserRow[]>("/users", filters),
    adminGet<{ overview: AdminOverview }>("/overview", filters),
  ]);
  const q = adminQuery(filters);

  return (
    <main className="adm-wrap"><div className="adm-inner">
      <header className="ui-page-head">
        <div>
          <div className="ui-eyebrow">Admin</div>
          <h1>Users</h1>
          <p>{users.length} account{users.length === 1 ? "" : "s"}. Activity columns count consenting users only, within the selected dates.</p>
        </div>
        <div className="ui-page-actions"><ExportLinks filters={filters} /></div>
      </header>
      <AdminFilters filters={filters} groups={overview.groups ?? []} action="/admin/users" />
      <div className="adm-table-wrap">
        <table className="adm-table">
          <thead>
            <tr>
              <th>User</th>
              <th>Group</th>
              <th>Analytics</th>
              <th>Last seen</th>
              <th className="num">Active days</th>
              <th className="num">Docs</th>
              <th className="num">Reading</th>
              <th className="num">Shown</th>
              <th className="num">Decisions (K/C/R)</th>
              <th className="num">Chat</th>
              <th className="num">Quiz avg</th>
            </tr>
          </thead>
          <tbody>
            {users.length === 0 && (
              <tr><td colSpan={11} className="adm-muted">No users match these filters.</td></tr>
            )}
            {users.map((u) => (
              <tr key={u.id}>
                <td>
                  <Link href={`/admin/users/${u.id}${q}`}>{u.email}</Link>
                  {u.role === "admin" && <span className="ui-badge ui-badge--sky adm-badge">admin</span>}
                </td>
                <td>{u.group_label || <span className="adm-muted">—</span>}</td>
                <td>{u.consented_at ? <span className="ui-badge ui-badge--green">opted in</span> : <span className="adm-muted">off</span>}</td>
                <td>{formatDay(u.last_seen_at)}</td>
                <td className="num">{u.active_days}</td>
                <td className="num">{u.documents}</td>
                <td className="num">{formatDuration(u.reading_ms)}</td>
                <td className="num">{u.suggestions_shown}</td>
                <td className="num">{u.decisions} <span className="adm-muted">({u.keep}/{u.change}/{u.reject})</span></td>
                <td className="num">{u.chat_questions}</td>
                <td className="num">{u.quiz_avg_pct == null ? <span className="adm-muted">—</span> : `${Math.round(u.quiz_avg_pct)}%`}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div></main>
  );
}
