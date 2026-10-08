// components/admin/Charts.tsx — dependency-free SVG charts for /admin.
// Server-renderable (no client JS). Kept deliberately small instead of adding
// a charting library to the bundle.

import type { DailyPoint } from "@/lib/admin";

export function Sparkline({
  data,
  field,
  label,
  format = (n: number) => String(n),
  summary = "total",
}: {
  data: DailyPoint[];
  field: keyof Omit<DailyPoint, "day">;
  label: string;
  format?: (n: number) => string;
  /** "total" sums the days; "peak" shows the busiest day (for distinct-user counts, where a sum is meaningless). */
  summary?: "total" | "peak";
}) {
  const w = 320;
  const h = 72;
  const values = data.map((d) => Number(d[field]) || 0);
  const max = Math.max(1, ...values);
  const step = data.length > 1 ? w / data.length : w;
  const total = values.reduce((a, b) => a + b, 0);
  return (
    <figure className="adm-chart">
      <figcaption>
        <span>{label}</span>
        <b>{summary === "peak" ? `peak ${format(max === 1 && total === 0 ? 0 : max)}` : format(total)}</b>
      </figcaption>
      {data.length === 0 ? (
        <p className="adm-muted">No data in this range.</p>
      ) : (
        <svg viewBox={`0 0 ${w} ${h}`} role="img" aria-label={`${label} per day, peak ${format(max)}`} preserveAspectRatio="none">
          {values.map((v, i) => {
            const bh = (v / max) * (h - 4);
            return (
              <rect key={data[i].day} x={i * step + 1} width={Math.max(1, step - 2)} y={h - bh} height={bh} className="adm-bar">
                <title>{`${data[i].day}: ${format(v)}`}</title>
              </rect>
            );
          })}
        </svg>
      )}
      {data.length > 0 && (
        <div className="adm-axis"><span>{data[0].day}</span><span>{data[data.length - 1].day}</span></div>
      )}
    </figure>
  );
}

export function StackedBar({ parts, label }: { parts: { name: string; value: number; tone: string }[]; label: string }) {
  const total = parts.reduce((a, p) => a + p.value, 0);
  return (
    <figure className="adm-chart">
      <figcaption>
        <span>{label}</span>
        <b>{total}</b>
      </figcaption>
      <div className="adm-stack" role="img" aria-label={parts.map((p) => `${p.name} ${p.value}`).join(", ")}>
        {total === 0
          ? <div className="adm-stack-empty" />
          : parts.map((p) => (p.value > 0 ? <div key={p.name} className={`adm-seg adm-seg--${p.tone}`} style={{ flexGrow: p.value }} title={`${p.name}: ${p.value}`} /> : null))}
      </div>
      <ul className="adm-legend">
        {parts.map((p) => (
          <li key={p.name}>
            <i className={`adm-seg--${p.tone}`} /> {p.name} <b>{p.value}</b>{" "}
            <span className="adm-muted">{total ? `${Math.round((p.value / total) * 100)}%` : ""}</span>
          </li>
        ))}
      </ul>
    </figure>
  );
}
