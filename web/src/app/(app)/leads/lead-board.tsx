"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import { moveLead } from "@/app/actions-leads";
import { MoveIcon } from "@/components/icons";
import { initials, interestLabel, isPast, moneyShort, relative, STATUS_BAR, STATUS_LABEL } from "@/lib/format";
import { LEAD_STATUSES, type Lead, type LeadStatus } from "@/lib/types";
import { cn } from "@/lib/utils";

/** The pipeline as columns. Drag a card to another column to change its status; the select on each card does the same
 * for keyboards and touch screens. Won opens the sale form instead, so a won lead always has a sale. */
export function LeadBoard({ leads }: { leads: Lead[] }) {
  const router = useRouter();
  const [moved, setMoved] = useState<Record<string, LeadStatus>>({});
  const [over, setOver] = useState<LeadStatus | null>(null);
  const [dragging, setDragging] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [, start] = useTransition();

  const statusOf = (l: Lead) => moved[l.id] ?? l.status;

  function move(lead: Lead, to: LeadStatus) {
    setOver(null);
    setDragging(null);
    if (statusOf(lead) === to) return;
    if (to === "won") {
      router.push(`/sales/new?lead=${lead.id}`);
      return;
    }
    let reason: string | undefined;
    if (to === "lost") {
      const answer = window.prompt(`Why was ${lead.name} lost? (optional)`, lead.lost_reason);
      if (answer === null) return;
      reason = answer.trim();
    }
    const before = statusOf(lead);
    setError("");
    setMoved((m) => ({ ...m, [lead.id]: to }));
    start(async () => {
      const res = await moveLead(lead.id, to, reason);
      if (res.error) {
        setMoved((m) => ({ ...m, [lead.id]: before }));
        setError(res.error);
      } else {
        router.refresh();
      }
    });
  }

  return (
    <>
      {error && <p role="alert" className="mb-3 rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
      <div className="grid auto-cols-[17rem] grid-flow-col gap-3 overflow-x-auto pb-3">
        {LEAD_STATUSES.map((s) => {
          const col = leads.filter((l) => statusOf(l) === s);
          const value = col.reduce((n, l) => n + l.expected_value, 0);
          return (
            <section
              key={s}
              aria-label={STATUS_LABEL[s]}
              onDragOver={(e) => {
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
                if (over !== s) setOver(s);
              }}
              onDragLeave={(e) => {
                if (!e.currentTarget.contains(e.relatedTarget as Node)) setOver(null);
              }}
              onDrop={(e) => {
                e.preventDefault();
                const lead = leads.find((l) => l.id === e.dataTransfer.getData("text/plain"));
                if (lead) move(lead, s);
              }}
              className={cn("flex min-h-64 flex-col rounded-2xl bg-muted/50 p-2.5 transition-colors", over === s && dragging && "bg-brand/10 ring-2 ring-brand/40")}
            >
              <h2 className="flex items-center gap-2 px-1.5 pb-2 text-xs font-semibold text-muted-foreground">
                <span className={cn("size-2 rounded-full", STATUS_BAR[s])} />
                {STATUS_LABEL[s]}
                <span className="ml-auto tabular-nums">{col.length}</span>
              </h2>
              {value > 0 && <p className="-mt-1 px-1.5 pb-2 text-xs text-muted-foreground tabular-nums">{moneyShort(value)}</p>}
              <ul className="flex-1 space-y-2">
                {col.map((l) => (
                  <li
                    key={l.id}
                    draggable
                    onDragStart={(e) => {
                      e.dataTransfer.setData("text/plain", l.id);
                      e.dataTransfer.effectAllowed = "move";
                      setDragging(l.id);
                    }}
                    onDragEnd={() => {
                      setDragging(null);
                      setOver(null);
                    }}
                    className={cn("group relative cursor-grab rounded-xl bg-card p-3 ring-1 ring-foreground/10 transition hover:ring-foreground/25 active:cursor-grabbing", dragging === l.id && "opacity-40")}
                  >
                    <Link href={`/leads/${l.id}`} draggable={false} className="block pr-6 outline-none focus-visible:underline">
                      <p className="truncate text-sm font-medium">{l.name}</p>
                      <p className="truncate text-xs text-muted-foreground">{l.company || l.email}</p>
                    </Link>
                    <p className="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                      <span className="truncate">{interestLabel(l.interest, l.plan_interest)}</span>
                      {l.expected_value > 0 && <span className="shrink-0 font-medium text-foreground tabular-nums">{moneyShort(l.expected_value)}</span>}
                    </p>
                    <div className="mt-2 flex items-center justify-between gap-2 text-xs">
                      {l.owner_id ? (
                        <span className="inline-flex min-w-0 items-center gap-1.5" title={l.owner_name}>
                          <span className="grid size-5 shrink-0 place-items-center rounded-full bg-muted text-[9px] font-semibold text-muted-foreground">{initials(l.owner_name)}</span>
                          <span className="truncate text-muted-foreground">{l.owner_name.split(" ")[0]}</span>
                        </span>
                      ) : (
                        <span className="text-muted-foreground">Unassigned</span>
                      )}
                      <span className={cn("shrink-0", isPast(l.next_follow_up_at) ? "font-medium text-destructive" : "text-muted-foreground")}>
                        {l.next_follow_up_at ? `Follow up ${relative(l.next_follow_up_at)}` : relative(l.created_at)}
                      </span>
                    </div>
                    <span className="absolute top-2 right-2 grid size-6 place-items-center rounded-full bg-muted text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100">
                      <MoveIcon className="size-3.5" />
                      <select
                      aria-label={`Move ${l.name} to another stage`}
                      value={s}
                      onChange={(e) => move(l, e.target.value as LeadStatus)}
                      className="absolute inset-0 size-full cursor-pointer opacity-0"
                    >
                      {LEAD_STATUSES.map((x) => <option key={x} value={x}>{STATUS_LABEL[x]}</option>)}
                    </select>
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          );
        })}
      </div>
    </>
  );
}
