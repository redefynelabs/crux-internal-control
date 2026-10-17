import Link from "next/link";
import { Card, EmptyState, PageHeader } from "@/components/ui";
import { getAudit } from "@/lib/api";
import { dateTime, relative } from "@/lib/format";

export const metadata = { title: "Activity" };

const ACTION: Record<string, string> = {
  customer_created: "added a customer",
  customer_updated: "changed customer details",
  entitlement_issued: "issued an entitlement",
  licence_changed: "changed a licence",
  activation_code_created: "made an activation code",
  activated: "activated an installation",
  suspended: "suspended an installation",
  resumed: "resumed an installation",
  revoked: "revoked a key",
  credential_replaced: "replaced a secret",
  release_published: "published a release",
  release_withdrawn: "withdrew a release",
};

export default async function ActivityPage() {
  const entries = await getAudit();
  return (
    <>
      <PageHeader title="Activity" description="Who changed what: licences, access, activation and releases. The newest 200 entries." />
      <Card className="overflow-hidden">
        {entries.length === 0 ? (
          <EmptyState title="Nothing recorded yet" text="Changes to licences, access and releases appear here." />
        ) : (
          <ul className="divide-y text-sm">
            {entries.map((e) => (
              <li key={e.id} className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 px-5 py-3">
                <span>
                  <strong className="font-medium">{e.actor_email || "The customer"}</strong> {ACTION[e.action] ?? e.action}
                  {e.installation_id && <> for <Link href={`/installations/${e.installation_id}`} className="font-medium hover:underline">{e.installation_name}</Link></>}
                  {typeof e.detail?.version === "string" && <span className="text-muted-foreground"> v{e.detail.version}</span>}
                  {typeof e.detail?.note === "string" && e.detail.note && <span className="text-muted-foreground"> · {e.detail.note}</span>}
                </span>
                <time dateTime={e.at} title={dateTime(e.at)} className="text-xs text-muted-foreground">{relative(e.at)}</time>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  );
}
