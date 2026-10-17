import Link from "next/link";
import { notFound } from "next/navigation";
import { rotateCredential } from "@/app/actions-entitlements";
import { ConfirmButton } from "@/components/confirm-button";
import { ArrowLeftIcon } from "@/components/icons";
import { AmcBadge, LICENCE_LABEL, StateBadge } from "@/components/installation-badges";
import { Badge, ButtonLink, Card, EmptyState, PageHeader } from "@/components/ui";
import { ApiError, getCatalog, getInstallation, getInstallationCredential, getMe } from "@/lib/api";
import { dateOnly, dateTime, relative } from "@/lib/format";
import { ActivationPanel } from "./activation-panel";
import { DeclineForm } from "./decline-form";
import { LicenceForm } from "./licence-form";
import { StatusForm } from "./status-form";

export const metadata = { title: "Customer" };

/** What to do about each state, in plain words. */
const NEXT: Record<string, string> = {
  awaiting_activation: "Make an activation code and give it to the customer. They enter it when they set up the ERP.",
  out_of_date: "The customer has not applied the newest file yet. It applies on their next check; nothing to do unless this stays for long.",
  unreachable: "Nothing has been heard from this installation for over a day. Check that it is running and can reach this control plane.",
  expired: "The subscription ended. Renew it under Licence and maintenance, or leave the ERP to go read-only after 14 days.",
  suspended: "Access is read-only until you resume it.",
  revoked: "The key is revoked. The installation receives nothing more.",
};

const ACTION: Record<string, string> = {
  customer_created: "Customer added",
  customer_updated: "Customer details changed",
  entitlement_issued: "Entitlement issued",
  licence_changed: "Licence changed",
  activation_code_created: "Activation code made",
  activated: "Activated by the customer",
  suspended: "Suspended",
  resumed: "Resumed",
  revoked: "Key revoked",
  credential_replaced: "Secret replaced",
};

const TONE: Record<string, string> = {
  pending: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200",
  approved: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
  declined: "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200",
  withdrawn: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200",
};

export default async function InstallationPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const me = await getMe();
  const admin = me.role === "admin";
  let detail;
  try {
    detail = await getInstallation(id);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) notFound();
    throw e;
  }
  const [{ catalog, can_issue, install_url }, credential] = await Promise.all([getCatalog(), admin ? getInstallationCredential(id) : null]);
  const { installation: inst, history, requests, audit, activation } = detail;
  const current = history[0]?.payload;
  const label = new Map([...catalog.core, ...catalog.apps.flatMap((a) => [{ name: a.name, label: a.label }, ...a.features])].map((f) => [f.name, f.label]));
  const template = catalog.templates.find((t) => t.id === current?.template)?.name ?? current?.template;

  return (
    <>
      <ButtonLink href="/installations" variant="ghost" size="sm" className="-ml-2 mb-3"><ArrowLeftIcon className="size-4" /> Customers</ButtonLink>
      <PageHeader
        title={inst.name}
        description={inst.kind === "demo" ? "A demo instance." : inst.notes || "An ERP installation."}
        actions={admin && can_issue ? <ButtonLink href={`/installations/${id}/issue`}>{current ? "Change apps and issue" : "Choose apps and issue"}</ButtonLink> : undefined}
      />
      {inst.kind === "customer" && (
        <Card className="mb-4 flex flex-wrap items-center gap-3 p-4">
          <StateBadge state={inst.state} text={inst.state_text} />
          <p className="min-w-0 flex-1 text-sm text-muted-foreground">{NEXT[inst.state] ?? inst.state_text}</p>
        </Card>
      )}
      {admin && !can_issue && <Card className="mb-4 border-amber-300 p-4 text-sm">This control plane has no signing key. Run <code>controlplane keygen</code>, set <code>CP_ENTITLEMENT_KEY_FILE</code> and restart to issue entitlements.</Card>}

      <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
        <div className="space-y-4">
          <Card className="p-5">
            <div className="flex items-center justify-between">
              <h2 className="font-medium">In force</h2>
              {history[0] && <ButtonLink href={`/installations/${id}/file/latest`} variant="secondary" size="sm" download>Download v{history[0].version}</ButtonLink>}
            </div>
            {!current ? (
              <p className="mt-3 text-sm text-muted-foreground">Nothing has been issued. The installation cannot be set up until it has a file.</p>
            ) : (
              <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                <div><dt className="text-xs text-muted-foreground">Template</dt><dd>{template}</dd></div>
                <div><dt className="text-xs text-muted-foreground">Issued</dt><dd>{dateTime(history[0].issued_at)}</dd></div>
                <div><dt className="text-xs text-muted-foreground">Licence</dt><dd>{LICENCE_LABEL[current.license?.type ?? (current.expires_at ? "subscription" : "")] ?? "Not set"}</dd></div>
                <div><dt className="text-xs text-muted-foreground">Subscription ends</dt><dd>{current.expires_at ? dateOnly(current.expires_at.slice(0, 10)) : "Never"}</dd></div>
                <div><dt className="text-xs text-muted-foreground">Updates included until</dt><dd className="flex items-center gap-2">{current.updates_until ? dateOnly(current.updates_until.slice(0, 10)) : "Not set"} <AmcBadge amc={inst.amc} /></dd></div>
                <div><dt className="text-xs text-muted-foreground">Words changed</dt><dd>{Object.keys(current.labels ?? {}).length || "None"}</dd></div>
                <div className="sm:col-span-2"><dt className="text-xs text-muted-foreground">Apps</dt><dd className="mt-1 flex flex-wrap gap-1.5">{current.apps.map((a) => <Badge key={a} className="bg-muted">{label.get(a) ?? a}</Badge>)}</dd></div>
                <div className="sm:col-span-2"><dt className="text-xs text-muted-foreground">Features</dt><dd className="mt-1 flex flex-wrap gap-1.5">{current.features.map((f) => <Badge key={f} className="bg-muted">{label.get(f) ?? f}</Badge>)}</dd></div>
              </dl>
            )}
          </Card>

          {inst.kind === "customer" && (
            <Card className="p-5">
              <h2 className="font-medium">Running</h2>
              <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                <div><dt className="text-xs text-muted-foreground">Last check-in</dt><dd>{inst.last_seen_at ? `${relative(inst.last_seen_at)} (${dateTime(inst.last_seen_at)})` : "Never"}</dd></div>
                <div><dt className="text-xs text-muted-foreground">Activated</dt><dd>{inst.activated_at ? dateTime(inst.activated_at) : "Not yet"}</dd></div>
                <div><dt className="text-xs text-muted-foreground">ERP version</dt><dd>{inst.server_version ? `v${inst.server_version}` : "Not reported"}{inst.server_build && <span className="text-muted-foreground"> · built {inst.server_build}</span>}</dd></div>
                <div><dt className="text-xs text-muted-foreground">Entitlement applied</dt><dd>{inst.applied_version != null ? `v${inst.applied_version}` : "Not reported"}{inst.version > 0 && inst.applied_version !== inst.version && <span className="text-muted-foreground"> · newest is v{inst.version}</span>}</dd></div>
              </dl>
            </Card>
          )}

          <Card className="overflow-hidden">
            <h2 className="px-5 pt-5 font-medium">Requests</h2>
            {requests.length === 0 ? <EmptyState title="No requests" text="What this customer asks for from their ERP shows here." /> : (
              <div className="mt-2 divide-y">
                {requests.map((r) => (
                  <div key={r.id} className="space-y-2 px-5 py-4">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <p className="text-sm"><code className="rounded bg-muted px-1.5 py-0.5 text-xs">{r.name}</code> <span className="text-muted-foreground">{label.get(r.name)}</span></p>
                      <Badge className={TONE[r.status]}>{r.status}</Badge>
                    </div>
                    {r.note && <p className="text-sm text-muted-foreground">{r.note}</p>}
                    {r.decided_note && <p className="text-sm">Note to the customer: {r.decided_note}</p>}
                    <p className="text-xs text-muted-foreground">{relative(r.requested_at)}</p>
                    {admin && r.status === "pending" && (
                      <div className="flex flex-wrap items-center gap-2 pt-1">
                        {can_issue && <ButtonLink href={`/installations/${id}/issue?approve=${r.id}`} size="sm">Approve</ButtonLink>}
                        <DeclineForm installationId={id} requestId={r.id} />
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </Card>

          <Card className="overflow-hidden">
            <h2 className="px-5 pt-5 font-medium">History</h2>
            {history.length === 0 ? <EmptyState title="Nothing issued yet" /> : (
              <ul className="mt-2 divide-y text-sm">
                {history.map((h) => (
                  <li key={h.version} className="flex items-center justify-between gap-3 px-5 py-3">
                    <span>v{h.version} <span className="text-muted-foreground">· {dateTime(h.issued_at)} · {h.payload.apps.length} apps, {h.payload.features.length} features</span></span>
                    <Link href={`/installations/${id}/file/${h.version}`} prefetch={false} className="text-muted-foreground hover:text-foreground hover:underline">Download</Link>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <Card className="overflow-hidden">
            <h2 className="px-5 pt-5 font-medium">Activity</h2>
            {audit.length === 0 ? <EmptyState title="Nothing recorded yet" /> : (
              <ul className="mt-2 divide-y text-sm">
                {audit.map((e) => (
                  <li key={e.id} className="flex flex-wrap items-baseline justify-between gap-x-3 px-5 py-3">
                    <span>{ACTION[e.action] ?? e.action}{typeof e.detail?.note === "string" && e.detail.note ? <span className="text-muted-foreground"> · {e.detail.note}</span> : null}</span>
                    <span className="text-xs text-muted-foreground">{e.actor_email || "The customer"} · {relative(e.at)}</span>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        <div className="space-y-4">
        {admin && inst.kind === "customer" && (
          <>
            <ActivationPanel installationId={id} activation={activation} hasFile={history.length > 0} activated={!!inst.activated_at} installUrl={install_url} />
            {history.length > 0 && <LicenceForm installationId={id} current={current} />}
            <StatusForm installation={inst} />
          </>
        )}
        {admin && credential && (
          <Card className="h-fit p-5">
            <h2 className="font-medium">Manual setup</h2>
            <p className="mt-1 text-xs text-muted-foreground">Only for an ERP set up before activation codes existed: put these in its environment. A new installation uses the activation code instead and needs none of this.</p>
            <pre className="mt-3 overflow-x-auto rounded-xl bg-muted p-3 text-xs leading-relaxed">{Object.entries(credential).map(([k, v]) => `${k}=${v}`).join("\n")}</pre>
            <p className="mt-2 text-xs text-muted-foreground">Also set <code>CONTROL_PLANE_URL</code> to this service&apos;s public address. Keep the secret private.</p>
            <form action={rotateCredential.bind(null, id)} className="mt-3">
              <ConfirmButton message="Replace the secret? The installation stops reaching the control plane until it has the new one." variant="secondary">Replace secret</ConfirmButton>
            </form>
          </Card>
        )}
        </div>
      </div>
    </>
  );
}
