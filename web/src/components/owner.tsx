import { initials } from "@/lib/format";
import type { Lead } from "@/lib/types";

/** The lead's owner as a small avatar and name, or "Unassigned". */
export function Owner({ lead }: { lead: Pick<Lead, "owner_id" | "owner_name"> }) {
  if (!lead.owner_id) return <span className="text-muted-foreground">Unassigned</span>;
  return (
    <span className="inline-flex items-center gap-2">
      <span className="grid size-6 place-items-center rounded-full bg-muted text-[10px] font-semibold text-muted-foreground">{initials(lead.owner_name)}</span>
      <span className="truncate">{lead.owner_name}</span>
    </span>
  );
}
