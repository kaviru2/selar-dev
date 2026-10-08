// admin/users/[id]/page.tsx — one user's performance proxies and timeline.

import Link from "next/link";
import { notFound } from "next/navigation";
import { GroupLabelEditor } from "@/components/admin/GroupLabelEditor";
import { AdminApiError, adminGet } from "@/lib/admin-server";
import { adminQuery, formatDay, formatDuration, parseAdminFilters, pct, type AdminUserRow, type TimelineEvent } from "@/lib/admin";

export const dynamic = "force-dynamic";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function describe(e: TimelineEvent): string {
  const p = e.props ?? {};
  const bits = Object.entries(p)
    .filter(([k]) => !k.endsWith("_id") && k !== "session_id")
    .map(([k, v]) => (k.endsWith("_ms")
      ? `${k.slice(0, -3).replace(/_/g, " ")}: ${Number(v) < 60_000 ? `${Math.round(Number(v) / 1000)}s` : formatDuration(Number(v))}`
      : `${k.replace(/_/g, " ")}: ${String(v)}`));
  return bits.join(" · ");
}

export default async function AdminUserPage({ params, searchParams }: {
  params: Promise<{ id: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { id } = await params;
  if (!UUID.test(id)) notFound();
  const filters = parseAdminFilters(await searchParams);
  let user: AdminUserRow;
  let timeline: TimelineEvent[];
  try {
    [user, timeline] = await Promise.all([
      adminGet<AdminUserRow>(`/users/${id}`, filters),
      adminGet<TimelineEvent[]>(`/users/${id}/timeline`, filters, { limit: "500" }),
    ]);
  } catch (err) {
    if (err instanceof AdminApiError && (err.status === 404 || err.status === 400)) notFound();
    throw err;
  }

  return (
    <main className="adm-wrap"><div className="adm-inner">
      <p><Link href={`/admin/users${adminQuery(filters)}`}>← All users</Link></p>
      <header className="ui-page-head">
        <div>
          <div className="ui-eyebrow">User</div>
          <h1>{user.email}</h1>
          <p>
            Joined {formatDay(user.created_at)} · {user.role}
            {" · "}
            {user.consented_at ? `opted in ${formatDay(user.consented_at)} (${user.consent_version})` : "analytics off: no per-user events are recorded"}
          </p>
        </div>
      </header>

      <section className="adm-kpis" aria-label="Performance">
        <div className="adm-kpi"><span>Group</span><GroupLabelEditor userId={user.id} initial={user.group_label} /></div>
        <div className="adm-kpi"><span>Reading time</span><b>{formatDuration(user.reading_ms)}</b><small>{user.active_days} active days</small></div>
        <div className="adm-kpi"><span>Suggestions shown → decided</span><b>{user.suggestions_shown} → {user.decisions}</b><small>keep {pct(user.keep, user.decisions)} · change {pct(user.change, user.decisions)} · reject {pct(user.reject, user.decisions)}</small></div>
        <div className="adm-kpi"><span>Documents</span><b>{user.documents}</b></div>
        <div className="adm-kpi"><span>Chat questions</span><b>{user.chat_questions}</b></div>
        <div className="adm-kpi"><span>Quiz</span><b>{user.quiz_avg_pct == null ? "—" : `${Math.round(user.quiz_avg_pct)}%`}</b><small>{user.quiz_attempts} submitted</small></div>
      </section>

      <h2 className="adm-h2">Timeline <span className="adm-muted adm-h2-sub">(newest first, up to 500 events in range)</span></h2>
      {timeline.length === 0 ? (
        <p className="adm-muted">No events in this range.</p>
      ) : (
        <ol className="adm-timeline">
          {timeline.map((e) => (
            <li key={e.id}>
              <time dateTime={e.occurred_at}>{e.occurred_at.replace("T", " ").slice(0, 19)}</time>
              <code>{e.event}</code>
              <span className="adm-muted">{describe(e)}</span>
            </li>
          ))}
        </ol>
      )}
    </div></main>
  );
}
