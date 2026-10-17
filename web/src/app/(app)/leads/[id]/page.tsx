import { notFound } from "next/navigation";
import { extendDemo } from "@/app/actions";
import { ArrowLeftIcon, ExternalIcon } from "@/components/icons";
import { Countdown } from "@/components/countdown";
import { Owner } from "@/components/owner";
import { SubmitButton } from "@/components/pending-button";
import { DemoStatusBadge, LeadStatusBadge } from "@/components/status-badge";
import { TaskItem } from "@/components/task-item";
import { ButtonLink, Card, Select } from "@/components/ui";
import { ApiError, getDemos, getLead, getMe, getTemplates, getUsers } from "@/lib/api";
import { initials, interestLabel, money, relative, rupees } from "@/lib/format";
import type { LeadEvent } from "@/lib/types";
import { Composer } from "./composer";
import { LeadForm } from "./lead-form";
import { Timeline } from "./timeline";

export const metadata = { title: "Lead" };

/** Where the lead came from: the first event that carries utm, referrer or landing page. */
function attribution(events: LeadEvent[]): [string, string][] {
  for (const e of events) {
    const a = e.detail?.attribution;
    if (a && typeof a === "object") {
      const rows = Object.entries(a as Record<string, unknown>).filter(([, v]) => typeof v === "string" && v).map(([k, v]) => [k.replace(/^utm_/, "").replace(/_/g, " "), String(v)] as [string, string]);
      if (rows.length) return rows;
    }
  }
  return [];
}

export default async function LeadPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let data;
  try {
    data = await getLead(id);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) notFound();
    throw e;
  }
  const { lead, events, tasks, emails, sale } = data;
  const [me, users, templates, allDemos] = await Promise.all([getMe(), getUsers(), getTemplates(), getDemos(undefined, 500)]);
  const demos = allDemos.filter((d) => d.lead_id === lead.id);
  const liveDemo = demos.find((d) => d.status === "active" && /^https?:\/\//.test(d.url));
  const origin = attribution(events);
  const open = tasks.filter((t) => !t.done_at);
  const closed = tasks.filter((t) => t.done_at).slice(0, 5);

  return (
    <>
      <ButtonLink href="/leads" variant="ghost" size="sm" className="-ml-2 mb-3"><ArrowLeftIcon className="size-4" /> Leads</ButtonLink>

      <div className="mb-6 flex flex-wrap items-center gap-4">
        <span className="grid size-12 place-items-center rounded-full bg-muted text-base font-semibold text-muted-foreground">{initials(lead.name)}</span>
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-2xl font-semibold tracking-tight">{lead.name}</h1>
          <p className="truncate text-sm text-muted-foreground">{[lead.company, `Added ${relative(lead.created_at)}`].filter(Boolean).join(" · ")}</p>
        </div>
        <div className="flex flex-wrap items-center gap-3 text-sm">
          <Owner lead={lead} />
          <LeadStatusBadge status={lead.status} />
          {sale ? (
            <ButtonLink href={`/sales/${sale.id}`} variant="secondary" size="sm">Sale · {rupees(sale.amount)}{sale.billing === "monthly" ? "/mo" : sale.billing === "yearly" ? "/yr" : ""}</ButtonLink>
          ) : lead.status !== "lost" ? (
            <ButtonLink href={`/sales/new?lead=${lead.id}`} variant={lead.status === "won" || lead.status === "proposal" ? "default" : "outline"} size="sm">Record sale</ButtonLink>
          ) : null}
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <Card className="p-5"><Composer lead={lead} me={me} users={users} templates={templates} demoUrl={liveDemo?.url ?? ""} /></Card>
          <Card className="p-5">
            <h2 className="font-medium">Timeline</h2>
            <Timeline events={events} emails={emails} />
          </Card>
        </div>

        <div className="space-y-4">
          <Card className="p-5">
            <h2 className="mb-4 font-medium">Details</h2>
            <LeadForm key={lead.updated_at} lead={lead} users={users} />
          </Card>

          <Card className="p-5">
            <h2 className="font-medium">Tasks</h2>
            {tasks.length === 0 ? (
              <p className="mt-3 text-sm text-muted-foreground">No follow-up set. Add one from the composer.</p>
            ) : (
              <ul className="mt-3 divide-y">
                {open.map((t) => <TaskItem key={t.id} task={t} leadId={lead.id} />)}
                {closed.map((t) => <TaskItem key={t.id} task={t} leadId={lead.id} />)}
              </ul>
            )}
          </Card>

          <Card className="p-5">
            <h2 className="font-medium">Contact</h2>
            <dl className="mt-3 space-y-3 text-sm">
              <div><dt className="text-xs text-muted-foreground">Email</dt><dd className="break-all"><a className="hover:underline" href={`mailto:${lead.email}`}>{lead.email}</a></dd></div>
              {lead.phone && <div><dt className="text-xs text-muted-foreground">Phone</dt><dd><a className="hover:underline" href={`tel:${lead.phone}`}>{lead.phone}</a></dd></div>}
              {lead.company && <div><dt className="text-xs text-muted-foreground">Company</dt><dd>{lead.company}</dd></div>}
              <div><dt className="text-xs text-muted-foreground">Interested in</dt><dd>{interestLabel(lead.interest, lead.plan_interest)}</dd></div>
              {lead.expected_value > 0 && <div><dt className="text-xs text-muted-foreground">Expected value</dt><dd>{money(lead.expected_value)}</dd></div>}
              <div><dt className="text-xs text-muted-foreground">Source</dt><dd className="capitalize">{lead.source}</dd></div>
            </dl>
          </Card>

          {origin.length > 0 && (
            <Card className="p-5">
              <h2 className="font-medium">Where they came from</h2>
              <dl className="mt-3 space-y-3 text-sm">
                {origin.map(([k, v]) => (
                  <div key={k}><dt className="text-xs text-muted-foreground capitalize">{k}</dt><dd className="break-all">{v}</dd></div>
                ))}
              </dl>
            </Card>
          )}

          <Card className="p-5">
            <h2 className="font-medium">Demos</h2>
            {demos.length === 0 ? (
              <p className="mt-3 text-sm text-muted-foreground">No demo yet.</p>
            ) : (
              <ul className="mt-3 divide-y">
                {demos.map((d) => (
                  <li key={d.id} className="py-2.5 first:pt-0 last:pb-0">
                    <div className="flex items-center justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate font-mono text-xs">{d.slug}</p>
                        <p className="text-xs text-muted-foreground">{d.status === "active" ? <Countdown until={d.expires_at} /> : relative(d.created_at)}</p>
                      </div>
                      <div className="flex items-center gap-2">
                        <DemoStatusBadge status={d.status} />
                        {d.status === "active" && /^https?:\/\//.test(d.url) && (
                          <a href={d.url} target="_blank" rel="noreferrer" aria-label="Open demo" className="text-muted-foreground hover:text-foreground"><ExternalIcon className="size-4" /></a>
                        )}
                      </div>
                    </div>
                    {d.status === "active" && (
                      <form action={extendDemo.bind(null, d.id)} className="mt-2 flex items-center gap-1.5">
                        <Select name="minutes" aria-label="Extend by" defaultValue="60" className="h-8 w-28 text-xs">
                          <option value="30">+30 min</option>
                          <option value="60">+1 hour</option>
                          <option value="120">+2 hours</option>
                          <option value="480">+8 hours</option>
                        </Select>
                        <SubmitButton variant="secondary" size="sm" pending="…">Extend</SubmitButton>
                      </form>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>
      </div>
    </>
  );
}
