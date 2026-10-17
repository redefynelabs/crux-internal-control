import Link from "next/link";
import { Countdown } from "@/components/countdown";
import { AlertIcon } from "@/components/icons";
import { DemoStatusBadge, LeadStatusBadge } from "@/components/status-badge";
import { Card, EmptyState, PageHeader } from "@/components/ui";
import { getDemos, getLeads, getMe, getStats, getTasks } from "@/lib/api";
import { dateTime, interestLabel, isPast, money, relative, STATUS_BAR, STATUS_LABEL } from "@/lib/format";
import { LEAD_STATUSES } from "@/lib/types";

export const metadata = { title: "Overview" };

function greeting() {
  const h = new Date().getHours();
  return h < 12 ? "Good morning" : h < 17 ? "Good afternoon" : "Good evening";
}

export default async function OverviewPage() {
  const [me, stats, unassigned, tasks, live] = await Promise.all([
    getMe(),
    getStats(),
    getLeads({ owner: "none", limit: 5 }),
    getTasks("me", "open", 50),
    getDemos("active", 20),
  ]);
  const pipeline = LEAD_STATUSES.filter((s) => s !== "lost");
  const total = LEAD_STATUSES.reduce((n, s) => n + (stats.leads[s] ?? 0), 0);
  const max = Math.max(1, ...pipeline.map((s) => stats.leads[s] ?? 0));
  const due = tasks.filter((t) => isPast(t.due_at) || new Date(t.due_at).toDateString() === new Date().toDateString());
  const tiles = [
    { label: "New this week", value: String(stats.new_this_week), hint: `${stats.leads.new ?? 0} waiting for a first reply`, href: "/leads?status=new" },
    { label: "Unassigned", value: String(stats.unassigned), hint: "Nobody owns these yet", href: "/leads?tab=unassigned" },
    { label: "Pipeline value", value: money(stats.pipeline_value), hint: "Open leads with an expected value", href: "/leads" },
    { label: "Won this month", value: String(stats.won_this_month), hint: stats.won_value_this_month ? money(stats.won_value_this_month) : "No value recorded", href: "/leads?status=won" },
  ];

  return (
    <>
      <PageHeader title={`${greeting()}, ${me.name.split(" ")[0]}`} description="What needs doing today, and how the pipeline looks." />

      {stats.emails_failed > 0 && (
        <Link href="/settings" className="mb-4 flex items-center gap-3 rounded-2xl bg-destructive/10 px-4 py-3 text-sm text-destructive ring-1 ring-destructive/20 hover:bg-destructive/15">
          <AlertIcon className="size-4 shrink-0" />
          <span>{stats.emails_failed} {stats.emails_failed === 1 ? "email" : "emails"} could not be sent. Check the email settings.</span>
        </Link>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {tiles.map((t) => (
          <Link key={t.label} href={t.href} className="group rounded-2xl outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50">
            <Card className="h-full p-5 transition-shadow group-hover:ring-foreground/25">
              <p className="text-sm text-muted-foreground">{t.label}</p>
              <p className="mt-2 text-3xl font-semibold tracking-tight tabular-nums">{t.value}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t.hint}</p>
            </Card>
          </Link>
        ))}
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card>
          <div className="flex items-center justify-between px-5 pt-5">
            <h2 className="font-medium">My follow-ups today</h2>
            <Link href="/tasks" className="text-sm text-muted-foreground hover:text-foreground">All tasks</Link>
          </div>
          {due.length === 0 ? (
            <EmptyState title="Nothing due" text="Tasks you add to a lead appear here when they are due." />
          ) : (
            <ul className="mt-2 divide-y px-2 pb-2">
              {due.slice(0, 5).map((t) => (
                <li key={t.id}>
                  <Link href={`/leads/${t.lead_id}`} className="flex items-center gap-3 rounded-xl px-3 py-3 hover:bg-muted/60">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{t.title}</p>
                      <p className="truncate text-xs text-muted-foreground">{t.lead_name}</p>
                    </div>
                    <span className={isPast(t.due_at) ? "text-xs font-medium text-destructive" : "text-xs text-muted-foreground"} title={dateTime(t.due_at)}>{isPast(t.due_at) ? "Overdue · " : ""}{relative(t.due_at)}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card>
          <div className="flex items-center justify-between px-5 pt-5">
            <h2 className="font-medium">New and unassigned</h2>
            <Link href="/leads?tab=unassigned" className="text-sm text-muted-foreground hover:text-foreground">View all</Link>
          </div>
          {unassigned.length === 0 ? (
            <EmptyState title="All leads have an owner" />
          ) : (
            <ul className="mt-2 divide-y px-2 pb-2">
              {unassigned.slice(0, 5).map((l) => (
                <li key={l.id}>
                  <Link href={`/leads/${l.id}`} className="flex items-center gap-3 rounded-xl px-3 py-3 hover:bg-muted/60">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{l.name}</p>
                      <p className="truncate text-xs text-muted-foreground">{[l.company, interestLabel(l.interest, l.plan_interest)].filter((v) => v && v !== "—").join(" · ") || l.email}</p>
                    </div>
                    <LeadStatusBadge status={l.status} />
                    <span className="hidden w-20 text-right text-xs text-muted-foreground sm:block">{relative(l.created_at)}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <Card className="mt-4 p-5">
        <div className="flex items-baseline justify-between">
          <h2 className="font-medium">Pipeline</h2>
          <p className="text-sm text-muted-foreground">{total} {total === 1 ? "lead" : "leads"} · {stats.leads.lost ?? 0} lost</p>
        </div>
        {total === 0 ? (
          <EmptyState title="No leads yet" text="Leads appear here as visitors ask for a demo or send a message from the website." />
        ) : (
          <ul className="mt-4 space-y-2.5">
            {pipeline.map((s) => {
              const n = stats.leads[s] ?? 0;
              return (
                <li key={s}>
                  <Link href={`/leads?status=${s}`} className="grid grid-cols-[6.5rem_1fr_2rem] items-center gap-3 text-sm hover:opacity-80">
                    <span className="text-muted-foreground">{STATUS_LABEL[s]}</span>
                    <span className="h-3 overflow-hidden rounded-full bg-muted" aria-hidden="true">
                      <span className={`block h-full rounded-full ${STATUS_BAR[s]}`} style={{ width: `${(n / max) * 100}%` }} />
                    </span>
                    <span className="text-right font-medium tabular-nums">{n}</span>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </Card>

      <Card className="mt-4">
        <div className="flex items-center justify-between px-5 pt-5">
          <h2 className="font-medium">Running demos</h2>
          <Link href="/demos" className="text-sm text-muted-foreground hover:text-foreground">Manage</Link>
        </div>
        {live.length === 0 ? (
          <EmptyState title="No demos running" text="A demo starts when a visitor asks for one." />
        ) : (
          <ul className="mt-2 divide-y px-2 pb-2">
            {live.slice(0, 6).map((d) => (
              <li key={d.id}>
                <Link href={`/leads/${d.lead_id}`} className="flex items-center gap-3 rounded-xl px-3 py-3 hover:bg-muted/60">
                  <p className="min-w-0 flex-1 truncate font-mono text-xs">{d.slug}</p>
                  <DemoStatusBadge status={d.status} />
                  <span className="w-20 text-right text-xs text-muted-foreground"><Countdown until={d.expires_at} /></span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  );
}
