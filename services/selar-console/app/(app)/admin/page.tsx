// admin/page.tsx — overview KPIs computed by server-side aggregate queries.

import { AdminFilters, ExportLinks } from "@/components/admin/AdminFilters";
import { Sparkline, StackedBar } from "@/components/admin/Charts";
import { adminGet } from "@/lib/admin-server";
import { formatDuration, parseAdminFilters, pct, type AdminOverview } from "@/lib/admin";

export const dynamic = "force-dynamic";

function Kpi({ label, value, sub }: { label: string; value: string | number; sub?: string }) {
  return (
    <div className="adm-kpi">
      <span>{label}</span>
      <b>{value}</b>
      {sub && <small>{sub}</small>}
    </div>
  );
}

export default async function AdminOverviewPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const filters = parseAdminFilters(await searchParams);
  const { analytics_enabled, overview: o } = await adminGet<{ analytics_enabled: boolean; overview: AdminOverview }>("/overview", filters);
  const daily = o.daily ?? [];
  const d = o.decisions;
  const anon = Object.entries(o.anonymous_counts ?? {}).sort((a, b) => b[1] - a[1]);

  return (
    <main className="adm-wrap"><div className="adm-inner">
      <header className="ui-page-head">
        <div>
          <div className="ui-eyebrow">Admin</div>
          <h1>Usage overview</h1>
          <p>
            {o.from.slice(0, 10)} to {new Date(new Date(o.to).getTime() - 1).toISOString().slice(0, 10)}
            {o.group ? ` · group “${o.group}”` : " · all users"}. Per-user figures include consenting users only.
          </p>
        </div>
        <div className="ui-page-actions"><ExportLinks filters={filters} /></div>
      </header>

      {!analytics_enabled && <p className="adm-warn">Analytics recording is switched off (ANALYTICS_ENABLED=false). Figures below are historical.</p>}

      <AdminFilters filters={filters} groups={o.groups ?? []} action="/admin" />

      <section className="adm-kpis" aria-label="Key figures">
        <Kpi label="Active users (1 d / 7 d)" value={`${o.active_users_last_1d} / ${o.active_users_last_7d}`} sub={`${o.active_users} in range`} />
        <Kpi label="Users" value={o.total_users} sub={`${o.consented_users} opted in · ${o.admin_users} admin`} />
        <Kpi label="Documents" value={o.documents} sub={`${o.documents_added} added in range`} />
        <Kpi label="Suggestions shown → decided" value={`${o.suggestions_shown} → ${d.total}`} sub={`decision rate ${pct(d.total, o.suggestions_shown)}`} />
        <Kpi label="Keep / change / reject" value={`${pct(d.keep, d.total)} / ${pct(d.change, d.total)} / ${pct(d.reject, d.total)}`} sub={`${d.retract + d.undo} retracted or undone`} />
        <Kpi label="Reading time" value={formatDuration(o.reading_ms)} sub={`${o.reading_sessions} reading sessions`} />
        <Kpi label="Chat questions" value={o.chat_questions} sub={`${o.chat_users} users asked`} />
        <Kpi label="Quiz submissions" value={o.quiz_submissions} />
      </section>

      <section className="adm-grid" aria-label="Charts">
        <Sparkline data={daily} field="active_users" label="Active users per day" summary="peak" />
        <Sparkline data={daily} field="reading_ms" label="Reading time per day" format={formatDuration} />
        <Sparkline data={daily} field="decisions" label="Decisions per day" />
        <Sparkline data={daily} field="chat_questions" label="Chat questions per day" />
        <StackedBar
          label="Decisions by outcome"
          parts={[
            { name: "Keep", value: d.keep, tone: "keep" },
            { name: "Change", value: d.change, tone: "change" },
            { name: "Reject", value: d.reject, tone: "reject" },
          ]}
        />
        <figure className="adm-chart">
          <figcaption><span>Anonymous event totals (all users, no ids)</span></figcaption>
          {anon.length === 0 ? (
            <p className="adm-muted">None in this range.</p>
          ) : (
            <ul className="adm-legend">
              {anon.slice(0, 8).map(([name, n]) => <li key={name}><code>{name}</code> <b>{n}</b></li>)}
            </ul>
          )}
        </figure>
      </section>

      <p className="adm-muted adm-foot">
        Anonymous totals count every user, including those who did not opt in, and are never linked to an account.
        These are interaction proxies, not learning outcomes. Reading time is visible time per page, capped at 30 minutes.
        Definitions are in docs/ANALYTICS.md.
      </p>
    </div></main>
  );
}
