import Link from "next/link";
import { PlusIcon, SearchIcon } from "@/components/icons";
import { Badge, Button, ButtonLink, Card, EmptyState, Input, PageHeader } from "@/components/ui";
import { getAnalytics, getSales } from "@/lib/api";
import { BILLING_LABEL, dateOnly, initials, rupees, SALE_PLAN_LABEL } from "@/lib/format";
import { cn } from "@/lib/utils";

export const metadata = { title: "Sales" };

const TABS = [
  { id: "active", label: "Active", query: { status: "active", billing: "recurring" } },
  { id: "one_time", label: "One-time", query: { status: "active", billing: "one_time" } },
  { id: "cancelled", label: "Cancelled", query: { status: "cancelled" } },
  { id: "all", label: "All", query: {} },
] as const;

export default async function SalesPage({ searchParams }: { searchParams: Promise<{ tab?: string; q?: string }> }) {
  const p = await searchParams;
  const tab = TABS.find((t) => t.id === p.tab) ?? TABS[0];
  const q = (p.q ?? "").trim();
  const [sales, a] = await Promise.all([getSales({ ...tab.query, q: q || undefined }), getAnalytics(6)]);
  const href = (t: string, query = q) => `/sales?tab=${t}${query ? `&q=${encodeURIComponent(query)}` : ""}`;
  const total = sales.reduce((n, s) => n + (tab.id === "active" ? s.mrr : s.amount), 0);

  return (
    <>
      <PageHeader title="Sales" description="Customers who bought, what they pay and for how long." actions={<ButtonLink href="/sales/new"><PlusIcon className="size-4" /> Record sale</ButtonLink>} />

      <div className="grid gap-4 sm:grid-cols-3">
        <Card className="p-5"><p className="text-sm text-muted-foreground">MRR</p><p className="mt-2 text-3xl font-semibold tracking-tight tabular-nums">{rupees(a.mrr)}</p><p className="mt-1 text-xs text-muted-foreground">{rupees(a.arr)} a year</p></Card>
        <Card className="p-5"><p className="text-sm text-muted-foreground">Customers</p><p className="mt-2 text-3xl font-semibold tracking-tight tabular-nums">{a.customers}</p><p className="mt-1 text-xs text-muted-foreground">{a.arpa ? `${rupees(a.arpa)} each, monthly` : "On a recurring plan"}</p></Card>
        <Card className="p-5"><p className="text-sm text-muted-foreground">New in 30 days</p><p className="mt-2 text-3xl font-semibold tracking-tight tabular-nums">{rupees(a.new_mrr_30)}</p><p className="mt-1 text-xs text-muted-foreground"><Link href="/analytics" className="hover:text-foreground hover:underline">See the analytics</Link></p></Card>
      </div>

      <div className="mt-6 mb-4 flex flex-wrap items-center justify-between gap-3">
        <nav aria-label="Sales views" className="flex gap-1.5">
          {TABS.map((t) => (
            <Link key={t.id} href={href(t.id)} aria-current={t.id === tab.id ? "page" : undefined} className={cn("inline-flex h-8 items-center rounded-4xl px-3 text-sm font-medium transition-colors", t.id === tab.id ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}>{t.label}</Link>
          ))}
        </nav>
        <form action="/sales" className="flex items-center gap-2">
          <input type="hidden" name="tab" value={tab.id} />
          <div className="relative w-full sm:w-64">
            <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input name="q" defaultValue={q} placeholder="Search customer or company" aria-label="Search sales" className="pl-9" />
          </div>
          <Button type="submit" variant="secondary">Search</Button>
        </form>
      </div>

      {sales.length === 0 ? (
        <Card><EmptyState title={q ? "No sales match" : "No sales here yet"} text={q ? "Try another search." : "Record a sale when a customer signs up, or close a lead as won."} /></Card>
      ) : (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-xs text-muted-foreground">
                  <th className="px-5 py-3 font-medium">Customer</th>
                  <th className="px-3 py-3 font-medium">Plan</th>
                  <th className="px-3 py-3 font-medium">Billed</th>
                  <th className="px-3 py-3 text-right font-medium">Amount</th>
                  <th className="px-3 py-3 text-right font-medium">MRR</th>
                  <th className="px-3 py-3 font-medium">Since</th>
                  <th className="px-3 py-3 font-medium">Sold by</th>
                  <th className="px-5 py-3 font-medium">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {sales.map((s) => (
                  <tr key={s.id} className="hover:bg-muted/50">
                    <td className="px-5 py-3">
                      <Link href={`/sales/${s.id}`} className="flex items-center gap-3 outline-none hover:underline focus-visible:underline">
                        <span className="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">{initials(s.customer)}</span>
                        <span className="min-w-0"><span className="block truncate font-medium">{s.customer}</span><span className="block truncate text-xs text-muted-foreground">{s.company || s.email}</span></span>
                      </Link>
                    </td>
                    <td className="px-3 py-3 text-muted-foreground">{SALE_PLAN_LABEL[s.plan] ?? "—"}</td>
                    <td className="px-3 py-3 whitespace-nowrap text-muted-foreground">{BILLING_LABEL[s.billing]}</td>
                    <td className="px-3 py-3 text-right tabular-nums">{rupees(s.amount)}</td>
                    <td className="px-3 py-3 text-right text-muted-foreground tabular-nums">{s.mrr ? rupees(s.mrr) : "—"}</td>
                    <td className="px-3 py-3 whitespace-nowrap text-muted-foreground">{dateOnly(s.started_on)}</td>
                    <td className="px-3 py-3 whitespace-nowrap text-muted-foreground">{s.owner_name || "—"}</td>
                    <td className="px-5 py-3"><Badge className={s.status === "active" ? "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200" : "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200"}>{s.status === "active" ? "Active" : "Cancelled"}</Badge></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
      {sales.length > 0 && <p className="mt-3 text-xs text-muted-foreground">{sales.length} shown · {tab.id === "active" ? `${rupees(total)} MRR` : `${rupees(total)} total`}</p>}
    </>
  );
}
