import Link from "next/link";
import { BarChart, Funnel, HBars, Legend, LineChart } from "@/components/charts";
import { Card, PageHeader } from "@/components/ui";
import { getAnalytics } from "@/lib/api";
import { INTEREST_LABEL, moneyShort, monthLabel, rupees, SALE_PLAN_LABEL } from "@/lib/format";
import { cn } from "@/lib/utils";

export const metadata = { title: "Analytics" };

const RANGES = [6, 12, 24] as const;

function Kpi({ label, value, hint, tone }: { label: string; value: string; hint?: string; tone?: "good" | "bad" }) {
  return (
    <Card className="p-5">
      <p className="text-sm text-muted-foreground">{label}</p>
      <p className="mt-2 text-3xl font-semibold tracking-tight tabular-nums">{value}</p>
      {hint && <p className={cn("mt-1 text-xs", tone === "good" ? "text-emerald-600 dark:text-emerald-400" : tone === "bad" ? "text-destructive" : "text-muted-foreground")}>{hint}</p>}
    </Card>
  );
}

function Panel({ title, note, children, className }: { title: string; note?: string; children: React.ReactNode; className?: string }) {
  return (
    <Card className={cn("p-5", className)}>
      <h2 className="font-medium">{title}</h2>
      {note && <p className="mt-0.5 text-xs text-muted-foreground">{note}</p>}
      <div className="mt-4">{children}</div>
    </Card>
  );
}

export default async function AnalyticsPage({ searchParams }: { searchParams: Promise<{ months?: string }> }) {
  const p = await searchParams;
  const months = (RANGES as readonly number[]).includes(Number(p.months)) ? Number(p.months) : 12;
  const a = await getAnalytics(months);
  const net = a.new_mrr_30 - a.churned_mrr_30;
  const label = (m: string) => monthLabel(m, months > 12);
  const first = a.series[0]?.mrr ?? 0;
  const growth = first > 0 ? Math.round(((a.mrr - first) / first) * 100) : null;

  return (
    <>
      <PageHeader
        title="Analytics"
        description="Recurring revenue, sales and how the pipeline is converting."
        actions={
          <nav aria-label="Range" className="flex gap-1.5">
            {RANGES.map((r) => (
              <Link key={r} href={r === 12 ? "/analytics" : `/analytics?months=${r}`} aria-current={r === months ? "page" : undefined} className={cn("inline-flex h-8 items-center rounded-4xl px-3 text-sm font-medium transition-colors", r === months ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}>{r} months</Link>
            ))}
          </nav>
        }
      />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Kpi label="MRR" value={rupees(a.mrr)} hint={growth === null ? "Monthly recurring revenue" : `${growth >= 0 ? "+" : ""}${growth}% over ${months} months`} tone={growth !== null && growth >= 0 ? "good" : growth !== null ? "bad" : undefined} />
        <Kpi label="ARR" value={rupees(a.arr)} hint="MRR × 12" />
        <Kpi label="Customers" value={String(a.customers)} hint={a.arpa ? `${rupees(a.arpa)} per customer a month` : "On a recurring plan"} />
        <Kpi label="Net new MRR, 30 days" value={`${net >= 0 ? "+" : "−"}${rupees(Math.abs(net))}`} hint={`${rupees(a.new_mrr_30)} new · ${rupees(a.churned_mrr_30)} lost`} tone={net >= 0 ? "good" : "bad"} />
      </div>

      <div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Kpi label="Churn, 90 days" value={`${a.churn_rate_90}%`} hint="Customers lost of those at the start" tone={a.churn_rate_90 > 10 ? "bad" : undefined} />
        <Kpi label="Win rate" value={`${a.win_rate}%`} hint="Won of all closed leads" />
        <Kpi label="Average deal" value={a.avg_deal ? rupees(a.avg_deal) : "—"} hint="Yearly value of a recurring sale" />
        <Kpi label="One-time revenue, 12 months" value={rupees(a.one_time_12m)} hint="Licences and set-up work" />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Panel title="Monthly recurring revenue" note="Recurring revenue at the end of each month.">
          <LineChart points={a.series.map((s) => ({ label: label(s.month), value: s.mrr }))} format={moneyShort} label="Monthly recurring revenue by month" />
        </Panel>
        <Panel title="New and lost MRR" note="What was added and what left each month.">
          <BarChart
            groups={a.series.map((s) => ({ label: label(s.month), values: [s.new_mrr, s.churned_mrr] }))}
            series={[{ name: "New MRR", color: "fill-emerald-500" }, { name: "Lost MRR", color: "fill-rose-500" }]}
            format={moneyShort}
            label="New and lost MRR by month"
          />
          <div className="mt-2"><Legend items={[{ name: "New MRR", color: "bg-emerald-500" }, { name: "Lost MRR", color: "bg-rose-500" }]} /></div>
        </Panel>
        <Panel title="Customers" note="Recurring customers at the end of each month.">
          <LineChart points={a.series.map((s) => ({ label: label(s.month), value: s.customers }))} format={(v) => String(Math.round(v))} label="Customers by month" integer />
        </Panel>
        <Panel title="One-time sales" note="Licences and set-up work sold each month.">
          <BarChart groups={a.series.map((s) => ({ label: label(s.month), values: [s.one_time] }))} series={[{ name: "One-time", color: "fill-violet-500" }]} format={moneyShort} label="One-time revenue by month" />
        </Panel>
        <Panel title="Leads, demos and sales" note="How many came in, tried a demo and bought each month.">
          <BarChart
            groups={a.series.map((s) => ({ label: label(s.month), values: [s.new_leads, s.demos, s.won] }))}
            series={[{ name: "New leads", color: "fill-blue-500" }, { name: "Demos", color: "fill-amber-500" }, { name: "Sales", color: "fill-emerald-500" }]}
            format={(v) => String(Math.round(v))}
            label="Leads, demos and sales by month"
          />
          <div className="mt-2"><Legend items={[{ name: "New leads", color: "bg-blue-500" }, { name: "Demos", color: "bg-amber-500" }, { name: "Sales", color: "bg-emerald-500" }]} /></div>
        </Panel>
        <Panel title="Conversion funnel" note="How far leads got, counting ones later lost.">
          <Funnel steps={a.funnel} />
        </Panel>
        <Panel title="Revenue by plan" note="Monthly recurring revenue per plan.">
          <HBars rows={a.plans.map((r) => ({ label: SALE_PLAN_LABEL[r.plan] ?? (r.plan || "Not set"), value: r.mrr, hint: `${r.customers} ${r.customers === 1 ? "customer" : "customers"}` }))} format={rupees} empty="No recurring sales yet" />
        </Panel>
        <Panel title="Where leads come from" note="Leads and how many were won, by source.">
          <HBars rows={a.sources.map((r) => ({ label: r.source === "website" ? "Demo form" : r.source === "website-contact" ? "Contact form" : (INTEREST_LABEL[r.source] ?? r.source), value: r.leads, hint: `${r.won} won` }))} format={(v) => String(v)} color="bg-blue-500" />
        </Panel>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel title="Pipeline">
          <dl className="space-y-3 text-sm">
            <div className="flex justify-between"><dt className="text-muted-foreground">Open leads</dt><dd className="tabular-nums">{a.open_leads}</dd></div>
            <div className="flex justify-between"><dt className="text-muted-foreground">Qualified and proposal value</dt><dd className="tabular-nums">{rupees(a.pipeline_value)}</dd></div>
            <div className="flex justify-between"><dt className="text-muted-foreground">Weighted by stage</dt><dd className="tabular-nums">{rupees(a.weighted_pipeline)}</dd></div>
            <div className="flex justify-between"><dt className="text-muted-foreground">Days from lead to sale</dt><dd className="tabular-nums">{a.avg_days_to_close || "—"}</dd></div>
          </dl>
        </Panel>
        <Panel title="Follow-ups, 30 days">
          <dl className="space-y-3 text-sm">
            <div className="flex justify-between"><dt className="text-muted-foreground">Tasks completed</dt><dd className="tabular-nums">{a.tasks_done_30}</dd></div>
            <div className="flex justify-between"><dt className="text-muted-foreground">Finished on time</dt><dd className="tabular-nums">{a.tasks_done_30 ? `${a.tasks_on_time_30}%` : "—"}</dd></div>
          </dl>
        </Panel>
        <Panel title="Team" className="lg:col-span-1">
          {a.owners.length === 0 ? (
            <p className="text-sm text-muted-foreground">Nobody on the team yet.</p>
          ) : (
            <ul className="divide-y text-sm">
              {a.owners.map((o) => (
                <li key={o.id} className="flex items-center justify-between gap-3 py-2 first:pt-0 last:pb-0">
                  <span className="min-w-0 truncate font-medium">{o.name}</span>
                  <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{o.leads} leads · {o.won} won · {moneyShort(o.sold_mrr)} MRR{o.overdue_tasks > 0 && <span className="text-destructive"> · {o.overdue_tasks} overdue</span>}</span>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>
    </>
  );
}
