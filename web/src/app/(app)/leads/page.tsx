import Link from "next/link";
import { BoardIcon, ListIcon, PlusIcon, SearchIcon } from "@/components/icons";
import { Owner } from "@/components/owner";
import { LeadStatusBadge } from "@/components/status-badge";
import { Button, ButtonLink, Card, EmptyState, Input, PageHeader, Select } from "@/components/ui";
import { getLeads, getStats, getUsers, type LeadFilter } from "@/lib/api";
import { dateTime, initials, interestLabel, isPast, money, relative, STATUS_LABEL } from "@/lib/format";
import { LEAD_STATUSES, type Lead } from "@/lib/types";
import { LeadBoard } from "./lead-board";
import { cn } from "@/lib/utils";

export const metadata = { title: "Leads" };

const TABS = [
  { id: "all", label: "All" },
  { id: "mine", label: "Mine" },
  { id: "unassigned", label: "Unassigned" },
  { id: "due", label: "Follow-up due" },
  { id: "cloud", label: "Cloud requests" },
] as const;
type Tab = (typeof TABS)[number]["id"];

type Params = { tab?: string; status?: string; q?: string; owner?: string; view?: string; sort?: string };

const SORTS = [
  { id: "new", label: "Newest first" },
  { id: "value", label: "Highest value" },
  { id: "follow", label: "Follow-up soonest" },
  { id: "stale", label: "Longest without contact" },
  { id: "name", label: "Name" },
] as const;

const time = (iso: string | null, fallback: number) => (iso ? new Date(iso).getTime() : fallback);

function sortLeads(leads: Lead[], sort: string): Lead[] {
  const by: Record<string, (a: Lead, b: Lead) => number> = {
    value: (a, b) => b.expected_value - a.expected_value,
    follow: (a, b) => time(a.next_follow_up_at, Infinity) - time(b.next_follow_up_at, Infinity),
    stale: (a, b) => time(a.last_contact_at, -Infinity) - time(b.last_contact_at, -Infinity),
    name: (a, b) => a.name.localeCompare(b.name),
  };
  return by[sort] ? [...leads].sort(by[sort]) : leads;
}

async function fetchLeads(tab: Tab, status: string | undefined, owner: string | undefined, q: string): Promise<Lead[]> {
  const base: LeadFilter = { status, q: q || undefined, owner: owner || undefined, limit: 500 };
  if (tab === "mine") base.owner = "me";
  if (tab === "unassigned") base.owner = "none";
  if (tab === "cloud") base.interest = "cloud";
  if (tab === "due") {
    const [over, today] = await Promise.all([getLeads({ ...base, due: "overdue" }), getLeads({ ...base, due: "today" })]);
    const seen = new Set<string>();
    return [...over, ...today].filter((l) => !seen.has(l.id) && seen.add(l.id));
  }
  return getLeads(base);
}

export default async function LeadsPage({ searchParams }: { searchParams: Promise<Params> }) {
  const p = await searchParams;
  const tab: Tab = TABS.some((t) => t.id === p.tab) ? (p.tab as Tab) : "all";
  const status = (LEAD_STATUSES as readonly string[]).includes(p.status ?? "") ? p.status : undefined;
  const q = (p.q ?? "").trim();
  const board = p.view === "board";
  const sort = SORTS.some((x) => x.id === p.sort) ? p.sort! : "new";
  const [stats, users, fetched] = await Promise.all([getStats(), getUsers(), fetchLeads(tab, status, p.owner, q)]);
  const leads = sortLeads(fetched, sort);
  const total = LEAD_STATUSES.reduce((n, s) => n + (stats.leads[s] ?? 0), 0);

  const href = (over: Partial<Params>) => {
    const next: Params = { tab: tab === "all" ? undefined : tab, status, q: q || undefined, owner: p.owner || undefined, view: board ? "board" : undefined, sort: sort === "new" ? undefined : sort, ...over };
    const qs = Object.entries(next).filter(([, v]) => v).map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`).join("&");
    return `/leads${qs ? `?${qs}` : ""}`;
  };

  return (
    <>
      <PageHeader
        title="Leads"
        description="Everyone who asked for a demo, a cloud plan or wrote to sales."
        actions={<ButtonLink href="/leads/new"><PlusIcon className="size-4" /> Add lead</ButtonLink>}
      />

      <dl className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        {[
          { label: "Open leads", value: String(total - (stats.leads.won ?? 0) - (stats.leads.lost ?? 0)), href: href({ status: undefined, tab: undefined }) },
          { label: "Unassigned", value: String(stats.unassigned), href: href({ tab: "unassigned", status: undefined }) },
          { label: "Follow-ups overdue", value: String(stats.overdue_tasks), href: href({ tab: "due", status: undefined }), alert: stats.overdue_tasks > 0 },
          { label: "Pipeline value", value: money(stats.pipeline_value) === "—" ? "₹0" : money(stats.pipeline_value), href: href({ status: "qualified", tab: undefined }) },
        ].map((k) => (
          <Link key={k.label} href={k.href} className="rounded-2xl bg-card px-4 py-3 ring-1 ring-foreground/10 transition-shadow hover:ring-foreground/25">
            <dt className="text-xs text-muted-foreground">{k.label}</dt>
            <dd className={cn("mt-0.5 text-xl font-semibold tabular-nums", k.alert && "text-destructive")}>{k.value}</dd>
          </Link>
        ))}
      </dl>

      <nav aria-label="Lead views" className="mb-3 flex gap-1 overflow-x-auto border-b">
        {TABS.map((t) => (
          <Link
            key={t.id}
            href={href({ tab: t.id === "all" ? undefined : t.id })}
            aria-current={tab === t.id ? "page" : undefined}
            className={cn("-mb-px shrink-0 border-b-2 px-3 py-2 text-sm font-medium transition-colors", tab === t.id ? "border-primary text-foreground" : "border-transparent text-muted-foreground hover:text-foreground")}
          >
            {t.label}
          </Link>
        ))}
      </nav>

      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <nav aria-label="Filter by status" className="flex flex-wrap gap-1.5">
          {[{ id: undefined, label: "All", n: total }, ...LEAD_STATUSES.map((s) => ({ id: s as string | undefined, label: STATUS_LABEL[s], n: stats.leads[s] ?? 0 }))].map((f) => (
            <Link
              key={f.label}
              href={href({ status: f.id })}
              aria-current={status === f.id ? "page" : undefined}
              className={cn("inline-flex h-8 items-center gap-1.5 rounded-4xl px-3 text-sm font-medium transition-colors", status === f.id ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}
            >
              {f.label}
              <span className={cn("text-xs tabular-nums", status === f.id ? "opacity-70" : "opacity-60")}>{f.n}</span>
            </Link>
          ))}
        </nav>
        <div className="flex items-center gap-2">
          <Link href={href({ view: undefined })} aria-label="List view" aria-current={!board ? "page" : undefined} className={cn("grid size-8 place-items-center rounded-4xl", !board ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}><ListIcon className="size-4" /></Link>
          <Link href={href({ view: "board" })} aria-label="Board view" aria-current={board ? "page" : undefined} className={cn("grid size-8 place-items-center rounded-4xl", board ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}><BoardIcon className="size-4" /></Link>
        </div>
      </div>

      <form action="/leads" className="mb-4 flex flex-wrap items-center gap-2">
        {tab !== "all" && <input type="hidden" name="tab" value={tab} />}
        {status && <input type="hidden" name="status" value={status} />}
        {board && <input type="hidden" name="view" value="board" />}
        <div className="relative w-full sm:w-72">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input name="q" defaultValue={q} placeholder="Search name, email, company" aria-label="Search leads" className="pl-9" />
        </div>
        <Select name="owner" defaultValue={p.owner ?? ""} aria-label="Owner" className="w-full sm:w-48">
          <option value="">Any owner</option>
          <option value="none">Unassigned</option>
          {users.filter((u) => u.active).map((u) => (
            <option key={u.id} value={u.id}>{u.name}</option>
          ))}
        </Select>
        <Select name="sort" defaultValue={sort} aria-label="Sort by" className="w-full sm:w-52">
          {SORTS.map((x) => <option key={x.id} value={x.id}>{x.label}</option>)}
        </Select>
        <Button type="submit" variant="secondary">Apply</Button>
        {(q || p.owner) && <Link href={href({ q: undefined, owner: undefined })} className="text-sm text-muted-foreground hover:text-foreground">Clear</Link>}
      </form>

      {leads.length === 0 ? (
        <Card><EmptyState title={q || status || p.owner || tab !== "all" ? "No leads match" : "No leads yet"} text={q || status || p.owner || tab !== "all" ? "Try another view, status or search." : "They appear here as visitors use the website forms."} /></Card>
      ) : board ? (
        <LeadBoard leads={leads} />
      ) : (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-xs text-muted-foreground">
                  <th className="px-5 py-3 font-medium">Lead</th>
                  <th className="px-3 py-3 font-medium">Company</th>
                  <th className="px-3 py-3 font-medium">Status</th>
                  <th className="px-3 py-3 font-medium">Interest</th>
                  <th className="px-3 py-3 font-medium">Owner</th>
                  <th className="px-3 py-3 font-medium">Next follow-up</th>
                  <th className="px-3 py-3 font-medium">Last contact</th>
                  <th className="px-5 py-3 text-right font-medium">Created</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {leads.map((l) => (
                  <tr key={l.id} className="hover:bg-muted/50">
                    <td className="px-5 py-3">
                      <Link href={`/leads/${l.id}`} className="flex items-center gap-3 outline-none hover:underline focus-visible:underline">
                        <span className="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">{initials(l.name)}</span>
                        <span className="min-w-0">
                          <span className="block truncate font-medium">{l.name}</span>
                          <span className="block truncate text-xs text-muted-foreground">{l.email}</span>
                        </span>
                      </Link>
                    </td>
                    <td className="px-3 py-3 text-muted-foreground">{l.company || "—"}</td>
                    <td className="px-3 py-3"><LeadStatusBadge status={l.status} /></td>
                    <td className="px-3 py-3 whitespace-nowrap text-muted-foreground">{interestLabel(l.interest, l.plan_interest)}{l.expected_value > 0 && <span className="block text-xs">{money(l.expected_value)}</span>}</td>
                    <td className="px-3 py-3 whitespace-nowrap"><Owner lead={l} /></td>
                    <td className={cn("px-3 py-3 whitespace-nowrap", isPast(l.next_follow_up_at) ? "font-medium text-destructive" : "text-muted-foreground")} title={l.next_follow_up_at ? dateTime(l.next_follow_up_at) : undefined}>{l.next_follow_up_at ? relative(l.next_follow_up_at) : "—"}</td>
                    <td className="px-3 py-3 whitespace-nowrap text-muted-foreground" title={l.last_contact_at ? dateTime(l.last_contact_at) : undefined}>{l.last_contact_at ? relative(l.last_contact_at) : "—"}</td>
                    <td className="px-5 py-3 text-right whitespace-nowrap text-muted-foreground" title={dateTime(l.created_at)}>{relative(l.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
      {leads.length > 0 && <p className="mt-3 text-xs text-muted-foreground">{leads.length} shown</p>}
    </>
  );
}
