import { cn } from "@/lib/utils";

// Small SVG charts drawn here so the app has no chart dependency. They render on the server and carry a <title> on every
// mark, so hovering shows the exact figure and screen readers get the data from the figure's text alternative.

const W = 640;
const H = 240;
const PAD = { top: 12, right: 12, bottom: 28, left: 52 };

/** A round upper bound for an axis: 1, 2, 2.5, 5 or 10 times a power of ten. */
export function niceMax(max: number): number {
  if (max <= 0) return 1;
  const pow = 10 ** Math.floor(Math.log10(max));
  const f = max / pow;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10) * pow;
}

function Axes({ top, ticks, format, labels, n }: { top: number; ticks: number; format: (v: number) => string; labels: string[]; n: number }) {
  const ph = H - PAD.top - PAD.bottom;
  const pw = W - PAD.left - PAD.right;
  const every = Math.ceil(n / 8);
  return (
    <>
      {Array.from({ length: ticks + 1 }, (_, i) => {
        const y = PAD.top + ph - (ph * i) / ticks;
        return (
          <g key={i}>
            <line x1={PAD.left} x2={W - PAD.right} y1={y} y2={y} className="stroke-border" strokeDasharray={i === 0 ? undefined : "3 4"} />
            <text x={PAD.left - 8} y={y + 4} textAnchor="end" className="fill-muted-foreground text-[11px]">{format((top * i) / ticks)}</text>
          </g>
        );
      })}
      {labels.map((l, i) =>
        i % every === 0 || i === n - 1 ? (
          <text key={i} x={PAD.left + (n === 1 ? pw / 2 : (pw * i) / (n - 1))} y={H - 8} textAnchor="middle" className="fill-muted-foreground text-[11px]">{l}</text>
        ) : null,
      )}
    </>
  );
}

export interface Point {
  label: string;
  value: number;
}

/** A line over time with a soft area under it. */
export function LineChart({ points, format, label, className, integer = false }: { points: Point[]; format: (v: number) => string; label: string; className?: string; integer?: boolean }) {
  const max = Math.max(...points.map((p) => p.value), 0);
  // Counts get whole-number gridlines; money gets round numbers.
  const ticks = integer ? Math.min(Math.max(Math.ceil(max), 1), 4) : 4;
  const top = integer ? Math.ceil(Math.max(max, 1) / ticks) * ticks : niceMax(max);
  const ph = H - PAD.top - PAD.bottom;
  const pw = W - PAD.left - PAD.right;
  const n = points.length;
  const x = (i: number) => PAD.left + (n === 1 ? pw / 2 : (pw * i) / (n - 1));
  const y = (v: number) => PAD.top + ph - (ph * v) / top;
  const line = points.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)} ${y(p.value).toFixed(1)}`).join(" ");
  const area = `${line} L${x(n - 1).toFixed(1)} ${PAD.top + ph} L${x(0).toFixed(1)} ${PAD.top + ph} Z`;
  return (
    <figure className={className}>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={label} className="h-auto w-full">
        <Axes top={top} ticks={ticks} format={format} labels={points.map((p) => p.label)} n={n} />
        <path d={area} className="fill-brand/10" />
        <path d={line} fill="none" className="stroke-brand" strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
        {points.map((p, i) => (
          <circle key={i} cx={x(i)} cy={y(p.value)} r={3.5} className="fill-card stroke-brand" strokeWidth={2}>
            <title>{`${p.label}: ${format(p.value)}`}</title>
          </circle>
        ))}
      </svg>
      <figcaption className="sr-only">{points.map((p) => `${p.label} ${format(p.value)}`).join(", ")}</figcaption>
    </figure>
  );
}

export interface BarGroup {
  label: string;
  values: number[];
}

/** Bars per month, one per series, side by side. */
export function BarChart({ groups, series, format, label, className }: { groups: BarGroup[]; series: { name: string; color: string }[]; format: (v: number) => string; label: string; className?: string }) {
  const top = niceMax(Math.max(...groups.flatMap((g) => g.values), 0));
  const ph = H - PAD.top - PAD.bottom;
  const pw = W - PAD.left - PAD.right;
  const n = groups.length;
  const slot = pw / n;
  const bw = Math.min(26, (slot * 0.72) / series.length);
  const every = Math.ceil(n / 8);
  return (
    <figure className={className}>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={label} className="h-auto w-full">
        {Array.from({ length: 5 }, (_, i) => {
          const yy = PAD.top + ph - (ph * i) / 4;
          return (
            <g key={i}>
              <line x1={PAD.left} x2={W - PAD.right} y1={yy} y2={yy} className="stroke-border" strokeDasharray={i === 0 ? undefined : "3 4"} />
              <text x={PAD.left - 8} y={yy + 4} textAnchor="end" className="fill-muted-foreground text-[11px]">{format((top * i) / 4)}</text>
            </g>
          );
        })}
        {groups.map((g, gi) => {
          const cx = PAD.left + slot * gi + slot / 2;
          const start = cx - (bw * series.length) / 2;
          return (
            <g key={gi}>
              {g.values.map((v, si) => {
                const h = (ph * v) / top;
                return (
                  <rect key={si} x={start + bw * si} y={PAD.top + ph - h} width={bw - 2} height={Math.max(h, v > 0 ? 1 : 0)} rx={3} className={series[si].color}>
                    <title>{`${g.label} · ${series[si].name}: ${format(v)}`}</title>
                  </rect>
                );
              })}
              {(gi % every === 0 || gi === n - 1) && <text x={cx} y={H - 8} textAnchor="middle" className="fill-muted-foreground text-[11px]">{g.label}</text>}
            </g>
          );
        })}
      </svg>
      <figcaption className="sr-only">{groups.map((g) => `${g.label}: ${g.values.map((v, i) => `${series[i].name} ${format(v)}`).join(", ")}`).join("; ")}</figcaption>
    </figure>
  );
}

export function Legend({ items }: { items: { name: string; color: string }[] }) {
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
      {items.map((i) => (
        <li key={i.name} className="flex items-center gap-1.5"><span className={cn("size-2.5 rounded-sm", i.color)} />{i.name}</li>
      ))}
    </ul>
  );
}

/** Horizontal bars, one row each: a name, a bar against the largest value and the figure. */
export function HBars({ rows, format, empty = "Nothing yet", color = "bg-brand" }: { rows: { label: string; value: number; hint?: string }[]; format: (v: number) => string; empty?: string; color?: string }) {
  const max = Math.max(1, ...rows.map((r) => r.value));
  if (rows.length === 0) return <p className="py-6 text-center text-sm text-muted-foreground">{empty}</p>;
  return (
    <ul className="space-y-3">
      {rows.map((r) => (
        <li key={r.label}>
          <div className="flex items-baseline justify-between gap-3 text-sm">
            <span className="truncate">{r.label}</span>
            <span className="shrink-0 tabular-nums">{format(r.value)}{r.hint && <span className="ml-2 text-xs text-muted-foreground">{r.hint}</span>}</span>
          </div>
          <div className="mt-1 h-2 rounded-full bg-muted"><div className={cn("h-2 rounded-full", color)} style={{ width: `${Math.max(2, (r.value / max) * 100)}%` }} /></div>
        </li>
      ))}
    </ul>
  );
}

/** A funnel: each stage's bar against the first, with the share kept from the stage before. */
export function Funnel({ steps }: { steps: { key: string; label: string; count: number }[] }) {
  const first = Math.max(1, steps[0]?.count ?? 1);
  return (
    <ol className="space-y-2.5">
      {steps.map((s, i) => {
        const prev = i === 0 ? 0 : steps[i - 1].count;
        return (
          <li key={s.key}>
            <div className="flex items-baseline justify-between gap-3 text-sm">
              <span>{s.label}</span>
              <span className="tabular-nums">{s.count}{i > 0 && prev > 0 && <span className="ml-2 text-xs text-muted-foreground">{Math.round((s.count / prev) * 100)}% of previous</span>}</span>
            </div>
            <div className="mt-1 h-6 rounded-lg bg-muted"><div className="h-6 rounded-lg bg-brand/80" style={{ width: `${Math.max(1.5, (s.count / first) * 100)}%`, opacity: 1 - i * 0.1 }} /></div>
          </li>
        );
      })}
    </ol>
  );
}
