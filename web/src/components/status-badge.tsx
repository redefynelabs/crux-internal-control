import { STATUS_LABEL, STATUS_TONE } from "@/lib/format";
import type { DemoStatus, LeadStatus } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Badge } from "./ui";

export function LeadStatusBadge({ status }: { status: LeadStatus }) {
  return <Badge className={STATUS_TONE[status] ?? STATUS_TONE.demo_ended}>{STATUS_LABEL[status] ?? status}</Badge>;
}

const DEMO_TONE: Record<DemoStatus, string> = {
  provisioning: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200",
  active: "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
  failed: "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200",
  expired: "bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200",
};

export function DemoStatusBadge({ status }: { status: DemoStatus }) {
  return (
    <Badge className={cn("capitalize", DEMO_TONE[status])}>
      {status === "active" && <span className="size-1.5 rounded-full bg-emerald-500" />}
      {status}
    </Badge>
  );
}
