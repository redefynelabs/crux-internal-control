import type { Installation, InstallationState } from "@/lib/types";
import { Badge } from "./ui";

const STATE: Record<InstallationState, { label: string; tone: string }> = {
  active: { label: "Active", tone: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200" },
  out_of_date: { label: "Out of date", tone: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200" },
  unreachable: { label: "Unreachable", tone: "bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-200" },
  awaiting_activation: { label: "Awaiting activation", tone: "bg-sky-100 text-sky-800 dark:bg-sky-950 dark:text-sky-200" },
  expired: { label: "Expired", tone: "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200" },
  suspended: { label: "Suspended", tone: "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200" },
  revoked: { label: "Revoked", tone: "bg-slate-200 text-slate-800 dark:bg-slate-800 dark:text-slate-100" },
  demo: { label: "Demo", tone: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200" },
};

export const STATE_LABEL = Object.fromEntries(Object.entries(STATE).map(([k, v]) => [k, v.label])) as Record<InstallationState, string>;

/** Where an installation stands. The title says why, so a colour is never the only signal. */
export function StateBadge({ state, text }: { state: InstallationState; text?: string }) {
  const s = STATE[state] ?? STATE.active;
  return <Badge className={s.tone} title={text}>{s.label}</Badge>;
}

const AMC: Record<Installation["amc"], { label: string; tone: string } | null> = {
  none: null,
  ok: { label: "AMC active", tone: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200" },
  ending: { label: "AMC ending", tone: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200" },
  lapsed: { label: "AMC lapsed", tone: "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200" },
};

export function AmcBadge({ amc }: { amc: Installation["amc"] }) {
  const a = AMC[amc];
  return a ? <Badge className={a.tone}>{a.label}</Badge> : null;
}

export const LICENCE_LABEL: Record<string, string> = { subscription: "Subscription", lifetime: "Lifetime" };
