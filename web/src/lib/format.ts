import type { LeadStatus } from "./types";

export const STATUS_LABEL: Record<LeadStatus, string> = {
  new: "New",
  demo_active: "Demo running",
  demo_ended: "Demo ended",
  contacted: "Contacted",
  qualified: "Qualified",
  proposal: "Proposal",
  won: "Won",
  lost: "Lost",
};

/** Soft tinted badge colours, one per meaning, with dark-mode counterparts (the ERP's status-tone approach). */
export const STATUS_TONE: Record<LeadStatus, string> = {
  new: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200",
  demo_active: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
  demo_ended: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200",
  contacted: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200",
  qualified: "bg-violet-100 text-violet-800 dark:bg-violet-950 dark:text-violet-200",
  proposal: "bg-cyan-100 text-cyan-800 dark:bg-cyan-950 dark:text-cyan-200",
  won: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-200",
  lost: "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200",
};

/** Bar colours for the pipeline chart. */
export const STATUS_BAR: Record<LeadStatus, string> = {
  new: "bg-blue-500",
  demo_active: "bg-emerald-500",
  demo_ended: "bg-slate-400",
  contacted: "bg-amber-500",
  qualified: "bg-violet-500",
  proposal: "bg-cyan-500",
  won: "bg-green-600",
  lost: "bg-rose-500",
};

const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });

/** "3 minutes ago", "in 2 hours". */
export function relative(iso: string, now = Date.now()): string {
  const diff = (new Date(iso).getTime() - now) / 1000;
  const steps: [Intl.RelativeTimeFormatUnit, number][] = [["day", 86400], ["hour", 3600], ["minute", 60]];
  for (const [unit, secs] of steps) {
    if (Math.abs(diff) >= secs) return rtf.format(Math.round(diff / secs), unit);
  }
  return Math.abs(diff) < 10 ? "just now" : rtf.format(Math.round(diff), "second");
}

export function dateTime(iso: string): string {
  return new Intl.DateTimeFormat("en-IN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(iso));
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  return ((parts[0]?.[0] ?? "?") + (parts.length > 1 ? parts[parts.length - 1][0] : "")).toUpperCase();
}

/** Rupees with Indian digit grouping; 0 is "—". */
export function money(n: number): string {
  return n > 0 ? `₹${new Intl.NumberFormat("en-IN", { maximumFractionDigits: 0 }).format(n)}` : "—";
}

/** Rupees compact for tight spaces and chart axes: 1.45L, 2.3Cr, 12.5K. 0 is "₹0". */
export function moneyShort(n: number): string {
  const a = Math.abs(n);
  const f = (v: number, unit: string) => `₹${(Math.round(v * 10) / 10).toString()}${unit}`;
  if (a >= 1e7) return f(n / 1e7, "Cr");
  if (a >= 1e5) return f(n / 1e5, "L");
  if (a >= 1e3) return f(n / 1e3, "K");
  return `₹${Math.round(n)}`;
}

/** Rupees in full, and ₹0 for nothing (money() shows a dash). */
export function rupees(n: number): string {
  return `₹${new Intl.NumberFormat("en-IN", { maximumFractionDigits: 0 }).format(n)}`;
}

/** "Mar" or "Mar 26" from a YYYY-MM month. */
export function monthLabel(ym: string, withYear = false): string {
  const [y, m] = ym.split("-").map(Number);
  const name = new Date(y, m - 1, 1).toLocaleString("en", { month: "short" });
  return withYear ? `${name} ${String(y).slice(2)}` : name;
}

/** A YYYY-MM-DD date as "12 Mar 2026". */
export function dateOnly(ymd: string): string {
  const [y, m, d] = ymd.split("-").map(Number);
  return new Intl.DateTimeFormat("en-IN", { dateStyle: "medium" }).format(new Date(y, m - 1, d));
}

export const BILLING_LABEL: Record<string, string> = { monthly: "Monthly", yearly: "Yearly", one_time: "One-time" };
export const SALE_PLAN_LABEL: Record<string, string> = { start: "Start", grow: "Grow", complete: "Complete", small: "Small (old)", business: "Business (old)", larger: "Larger teams", custom: "Custom" };

export const INTEREST_LABEL: Record<string, string> = { cloud: "Cloud", on_prem: "Own server", other: "Other" };
export const PLAN_LABEL: Record<string, string> = { start: "Start", grow: "Grow", complete: "Complete", small: "Small (old)", business: "Business (old)", larger: "Larger teams" };
export const ROLE_LABEL: Record<string, string> = { admin: "Admin", sales: "Sales" };

/** "Cloud · Business", "Own server", or "—". */
export function interestLabel(interest: string, plan: string): string {
  const parts = [INTEREST_LABEL[interest], PLAN_LABEL[plan]].filter(Boolean);
  return parts.length ? parts.join(" · ") : "—";
}

/** An ISO instant is before now. */
export function isPast(iso: string | null | undefined, now = Date.now()): boolean {
  return !!iso && new Date(iso).getTime() < now;
}

/** Value for <input type="datetime-local"> (local time). */
export function toLocalInput(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}
