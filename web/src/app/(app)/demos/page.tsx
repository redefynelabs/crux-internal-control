import Link from "next/link";
import { endDemo, extendDemo } from "@/app/actions";
import { ConfirmButton } from "@/components/confirm-button";
import { Countdown } from "@/components/countdown";
import { ExternalIcon } from "@/components/icons";
import { SubmitButton } from "@/components/pending-button";
import { DemoStatusBadge } from "@/components/status-badge";
import { Card, EmptyState, PageHeader, Select } from "@/components/ui";
import { getDemos, getLeads } from "@/lib/api";
import { dateTime, relative } from "@/lib/format";
import type { Demo } from "@/lib/types";

export const metadata = { title: "Demos" };

const EXTEND = [
  { minutes: 30, label: "+30 min" },
  { minutes: 60, label: "+1 hour" },
  { minutes: 120, label: "+2 hours" },
  { minutes: 480, label: "+8 hours" },
];

export default async function DemosPage() {
  const [demos, leads] = await Promise.all([getDemos(), getLeads()]);
  const lead = new Map(leads.map((l) => [l.id, l]));
  const live = demos.filter((d) => d.status === "active" || d.status === "provisioning");
  const past = demos.filter((d) => d.status === "expired" || d.status === "failed").slice(0, 25);

  const who = (d: Demo) => {
    const l = lead.get(d.lead_id);
    return l ? (
      <Link href={`/leads/${l.id}`} className="font-medium hover:underline">{l.name}</Link>
    ) : (
      <span className="text-muted-foreground">Unknown</span>
    );
  };

  return (
    <>
      <PageHeader title="Demos" description="Private demo instances. Each one is removed by itself when its time is up." />

      <Card className="overflow-hidden">
        <div className="flex items-center justify-between px-5 pt-5">
          <h2 className="font-medium">Running</h2>
          <p className="text-sm text-muted-foreground">{live.length} now</p>
        </div>
        {live.length === 0 ? (
          <EmptyState title="No demos running" text="A demo starts when a visitor asks for one on the website." />
        ) : (
          <div className="mt-2 overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left text-xs text-muted-foreground">
                  <th className="px-5 py-3 font-medium">Visitor</th>
                  <th className="px-3 py-3 font-medium">Instance</th>
                  <th className="px-3 py-3 font-medium">Status</th>
                  <th className="px-3 py-3 font-medium">Time left</th>
                  <th className="px-5 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {live.map((d) => (
                  <tr key={d.id}>
                    <td className="px-5 py-3">{who(d)}<p className="text-xs text-muted-foreground">Started {relative(d.created_at)}</p></td>
                    <td className="px-3 py-3">
                      <p className="font-mono text-xs">{d.slug}</p>
                      {/^https?:\/\//.test(d.url) && (
                        <a href={d.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">Open <ExternalIcon className="size-3" /></a>
                      )}
                    </td>
                    <td className="px-3 py-3"><DemoStatusBadge status={d.status} /></td>
                    <td className="px-3 py-3" title={dateTime(d.expires_at)}><Countdown until={d.expires_at} /></td>
                    <td className="px-5 py-3">
                      <div className="flex items-center justify-end gap-2">
                        {d.status === "active" && (
                          <form action={extendDemo.bind(null, d.id)} className="flex items-center gap-1.5">
                            <Select name="minutes" aria-label="Extend by" defaultValue="60" className="h-8 w-28 text-xs">
                              {EXTEND.map((e) => (
                                <option key={e.minutes} value={e.minutes}>{e.label}</option>
                              ))}
                            </Select>
                            <SubmitButton variant="secondary" size="sm" pending="…">Extend</SubmitButton>
                          </form>
                        )}
                        <form action={endDemo.bind(null, d.id)}>
                          <ConfirmButton message={`End the demo for ${lead.get(d.lead_id)?.name ?? d.slug} now? Its data is deleted.`}>End now</ConfirmButton>
                        </form>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card className="mt-4 overflow-hidden">
        <div className="px-5 pt-5"><h2 className="font-medium">Recently ended</h2></div>
        {past.length === 0 ? (
          <EmptyState title="Nothing yet" />
        ) : (
          <div className="mt-2 overflow-x-auto">
            <table className="w-full text-sm">
              <tbody className="divide-y">
                {past.map((d) => (
                  <tr key={d.id}>
                    <td className="px-5 py-3">{who(d)}</td>
                    <td className="px-3 py-3 font-mono text-xs text-muted-foreground">{d.slug}</td>
                    <td className="px-3 py-3"><DemoStatusBadge status={d.status} /></td>
                    <td className="px-3 py-3 text-xs text-muted-foreground">{d.error ? <span className="text-destructive">{d.error}</span> : ""}</td>
                    <td className="px-5 py-3 text-right text-xs text-muted-foreground" title={dateTime(d.ended_at ?? d.expires_at)}>{relative(d.ended_at ?? d.expires_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </>
  );
}
