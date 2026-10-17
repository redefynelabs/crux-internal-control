import Link from "next/link";
import { PlusIcon } from "@/components/icons";
import { AmcBadge, LICENCE_LABEL, STATE_LABEL, StateBadge } from "@/components/installation-badges";
import { Badge, ButtonLink, Card, EmptyState, PageHeader } from "@/components/ui";
import { getAccessRequests, getInstallations, getMe } from "@/lib/api";
import { dateOnly, relative } from "@/lib/format";
import type { Installation, InstallationState } from "@/lib/types";
import { cn } from "@/lib/utils";

export const metadata = { title: "Customers" };

type View = "customers" | "requests" | "demos";

function href(view: View, extra: Record<string, string> = {}) {
  const p = new URLSearchParams({ tab: view, ...extra });
  return `/installations?${p}`;
}

export default async function InstallationsPage({ searchParams }: { searchParams: Promise<{ tab?: string; state?: string; amc?: string }> }) {
  const p = await searchParams;
  const [me, installations, pending] = await Promise.all([getMe(), getInstallations(), getAccessRequests("pending")]);
  const tab: View = p.tab === "requests" || p.tab === "demos" ? p.tab : "customers";
  const customers = installations.filter((i) => i.kind === "customer");
  const demos = installations.filter((i) => i.kind === "demo");

  const count = (s: InstallationState) => customers.filter((i) => i.state === s).length;
  const amcSoon = customers.filter((i) => i.amc === "ending" || i.amc === "lapsed").length;
  const filtered = customers.filter((i) => (!p.state || i.state === p.state) && (!p.amc || (p.amc === "soon" ? i.amc === "ending" || i.amc === "lapsed" : i.amc === p.amc)));
  const rows = tab === "demos" ? demos : filtered;

  const tabs = [
    { id: "customers" as const, label: `Customers (${customers.length})` },
    { id: "requests" as const, label: pending.length ? `Requests (${pending.length})` : "Requests" },
    { id: "demos" as const, label: "Demos" },
  ];
  const tiles: { label: string; n: number; to: Record<string, string>; hot: boolean }[] = [
    { label: "Awaiting activation", n: count("awaiting_activation"), to: { state: "awaiting_activation" }, hot: false },
    { label: "Out of date", n: count("out_of_date"), to: { state: "out_of_date" }, hot: true },
    { label: "Unreachable", n: count("unreachable"), to: { state: "unreachable" }, hot: true },
    { label: "AMC ending or lapsed", n: amcSoon, to: { amc: "soon" }, hot: true },
  ];
  const active = p.state || p.amc;

  return (
    <>
      <PageHeader
        title="Customers"
        description="Every installation: its licence, the apps it may use, the version it runs and whether it is checking in."
        actions={me.role === "admin" ? <ButtonLink href="/installations/new"><PlusIcon className="size-4" /> Add customer</ButtonLink> : undefined}
      />
      <nav aria-label="Customer views" className="mb-4 flex gap-1.5">
        {tabs.map((t) => (
          <Link key={t.id} href={href(t.id)} aria-current={t.id === tab ? "page" : undefined} className={cn("inline-flex h-8 items-center rounded-4xl px-3 text-sm font-medium transition-colors", t.id === tab ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}>{t.label}</Link>
        ))}
      </nav>

      {tab === "customers" && customers.length > 0 && (
        <section aria-label="What needs attention" className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
          {tiles.map((t) => (
            <Link key={t.label} href={href("customers", t.to)} className="rounded-2xl bg-card p-4 ring-1 ring-foreground/10 transition-colors hover:bg-muted/50">
              <p className={cn("text-2xl font-semibold tabular-nums", t.hot && t.n > 0 && "text-amber-600 dark:text-amber-400")}>{t.n}</p>
              <p className="text-xs text-muted-foreground">{t.label}</p>
            </Link>
          ))}
        </section>
      )}
      {tab === "customers" && active && (
        <p className="mb-3 flex items-center gap-2 text-sm text-muted-foreground">
          Showing {p.state ? STATE_LABEL[p.state as InstallationState]?.toLowerCase() ?? p.state : "AMC ending or lapsed"} ({filtered.length}).
          <Link href={href("customers")} className="text-foreground underline">Show all</Link>
        </p>
      )}

      {tab === "requests" ? (
        pending.length === 0 ? (
          <Card><EmptyState title="No requests waiting" text="Customers ask for more from Settings > Apps in their ERP. Their requests arrive here." /></Card>
        ) : (
          <Card className="divide-y">
            {pending.map((r) => (
              <div key={`${r.installation_id}${r.id}`} className="flex flex-wrap items-start justify-between gap-3 px-5 py-4">
                <div className="min-w-0">
                  <p className="text-sm"><Link href={`/installations/${r.installation_id}`} className="font-medium hover:underline">{r.installation_name}</Link> asked for <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{r.name}</code></p>
                  {r.note && <p className="mt-1 text-sm text-muted-foreground">{r.note}</p>}
                  <p className="mt-1 text-xs text-muted-foreground">{relative(r.requested_at)}</p>
                </div>
                {me.role === "admin" && <ButtonLink href={`/installations/${r.installation_id}`} variant="secondary" size="sm">Review</ButtonLink>}
              </div>
            ))}
          </Card>
        )
      ) : rows.length === 0 ? (
        <Card>
          <EmptyState
            title={tab === "demos" ? "No demos yet" : active ? "Nothing matches" : "No customers yet"}
            text={tab === "demos" ? "Each demo started from the website gets an entitlement for the demo template." : active ? "No customer is in that state." : "Add a customer, choose what they get, issue the file and give them an activation code."}
          />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-xs text-muted-foreground">
                  <th scope="col" className="px-5 py-3 font-medium">Name</th>
                  <th scope="col" className="px-3 py-3 font-medium">State</th>
                  <th scope="col" className="px-3 py-3 font-medium">Licence</th>
                  <th scope="col" className="px-3 py-3 font-medium">Updates until</th>
                  <th scope="col" className="px-3 py-3 font-medium">Running</th>
                  <th scope="col" className="px-3 py-3 font-medium">Last seen</th>
                  <th scope="col" className="px-5 py-3 font-medium">Requests</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {rows.map((i) => <Row key={i.id} i={i} />)}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </>
  );
}

function Row({ i }: { i: Installation }) {
  const demo = i.kind === "demo";
  return (
    <tr className="hover:bg-muted/50">
      <td className="px-5 py-3">
        <Link href={`/installations/${i.id}`} className="font-medium hover:underline">{i.name}</Link>
        <p className="text-xs text-muted-foreground">{i.version ? `${i.app_count} apps, ${i.feature_count} features · file v${i.version}` : "No file issued"}</p>
      </td>
      <td className="px-3 py-3"><StateBadge state={i.state} text={i.state_text} /></td>
      <td className="px-3 py-3 whitespace-nowrap text-muted-foreground">
        {demo || !i.license_type ? "—" : (
          <>
            {LICENCE_LABEL[i.license_type]}
            {i.license_type === "subscription" && <span className="block text-xs">{i.expires_at ? `to ${dateOnly(i.expires_at.slice(0, 10))}` : "no end date"}</span>}
          </>
        )}
      </td>
      <td className="px-3 py-3 whitespace-nowrap">
        {i.updates_until ? <span className="flex flex-col items-start gap-1"><span className="text-muted-foreground">{dateOnly(i.updates_until.slice(0, 10))}</span><AmcBadge amc={i.amc} /></span> : <span className="text-muted-foreground">—</span>}
      </td>
      <td className="px-3 py-3 whitespace-nowrap text-muted-foreground tabular-nums">
        {i.server_version ? `v${i.server_version}` : "—"}
        {i.applied_version != null && <span className="block text-xs">file v{i.applied_version}</span>}
      </td>
      <td className="px-3 py-3 whitespace-nowrap text-muted-foreground">{i.last_seen_at ? relative(i.last_seen_at) : "Never"}</td>
      <td className="px-5 py-3">{i.pending_requests > 0 ? <Badge className="bg-brand/15 text-brand">{i.pending_requests} waiting</Badge> : <span className="text-muted-foreground">—</span>}</td>
    </tr>
  );
}
