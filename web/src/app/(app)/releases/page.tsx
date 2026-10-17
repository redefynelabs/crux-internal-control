import { withdrawRelease } from "@/app/actions-entitlements";
import { AdminOnly } from "@/components/admin-only";
import { ConfirmButton } from "@/components/confirm-button";
import { Badge, Card, EmptyState, PageHeader } from "@/components/ui";
import { getCatalog, getMe, getReleases } from "@/lib/api";
import { dateOnly } from "@/lib/format";
import { ReleaseForm } from "./release-form";

export const metadata = { title: "Releases" };

const CHANNEL: Record<string, string> = {
  stable: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
  security: "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200",
  beta: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200",
};

export default async function ReleasesPage() {
  const me = await getMe();
  const admin = me.role === "admin";
  const [releases, { can_issue }] = await Promise.all([getReleases(), getCatalog()]);
  return (
    <>
      <PageHeader title="Releases" description="The versions of the ERP you send out. A customer sees a release only when their maintenance (AMC) covers the day it was published." />
      <div className="grid gap-4 lg:grid-cols-[1fr_26rem]">
        <Card className="h-fit overflow-hidden">
          {releases.length === 0 ? (
            <EmptyState title="No release published" text="Publish the first version so customers can be offered updates." />
          ) : (
            <ul className="divide-y">
              {releases.map((r) => (
                <li key={r.id} className="space-y-1.5 px-5 py-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p className="flex flex-wrap items-center gap-2 font-medium">
                      v{r.version}
                      <Badge className={CHANNEL[r.channel]}>{r.channel}</Badge>
                      {r.has_migrations && <Badge className="bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200">Changes the database</Badge>}
                      {r.withdrawn_at && <Badge className="bg-slate-200 text-slate-800 dark:bg-slate-800 dark:text-slate-100">Withdrawn</Badge>}
                    </p>
                    <span className="text-xs text-muted-foreground">{dateOnly(r.published_at.slice(0, 10))} · {r.running} running</span>
                  </div>
                  {r.notes && <p className="text-sm whitespace-pre-line text-muted-foreground">{r.notes}</p>}
                  {r.min_from_version && <p className="text-xs text-muted-foreground">Upgrades from v{r.min_from_version} or newer.</p>}
                  {admin && !r.withdrawn_at && (
                    <form action={withdrawRelease.bind(null, r.id)}>
                      <ConfirmButton message="Stop offering this release? Customers already running it are not affected.">Withdraw</ConfirmButton>
                    </form>
                  )}
                </li>
              ))}
            </ul>
          )}
        </Card>
        {!admin ? <AdminOnly what="Publishing releases" /> : !can_issue ? (
          <Card className="h-fit p-5 text-sm">This control plane has no signing key, so releases cannot be signed. Run <code>controlplane keygen</code> and set <code>CP_ENTITLEMENT_KEY_FILE</code>.</Card>
        ) : <ReleaseForm />}
      </div>
    </>
  );
}
