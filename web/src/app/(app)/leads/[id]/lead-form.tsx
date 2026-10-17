"use client";

import { useActionState, useState } from "react";
import { saveLead } from "@/app/actions-leads";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input, Select, Textarea } from "@/components/ui";
import { INTEREST_LABEL, PLAN_LABEL, STATUS_LABEL } from "@/lib/format";
import { LEAD_STATUSES, type Lead, type User } from "@/lib/types";

export function LeadForm({ lead, users }: { lead: Lead; users: User[] }) {
  const [state, action] = useActionState<FormState, FormData>(saveLead.bind(null, lead.id), {});
  const [status, setStatus] = useState<string>(lead.status);
  // The current owner may be inactive now; keep them selectable so saving does not silently unassign.
  const owners = users.filter((u) => u.active || u.id === lead.owner_id);
  return (
    <form action={action} className="space-y-4">
      <Field id="status" label="Status">
        <Select id="status" name="status" value={status} onChange={(e) => setStatus(e.target.value)}>
          {LEAD_STATUSES.map((s) => <option key={s} value={s}>{STATUS_LABEL[s]}</option>)}
        </Select>
      </Field>
      {status === "lost" && (
        <Field id="lost_reason" label="Why was it lost?">
          <Input id="lost_reason" name="lost_reason" defaultValue={lead.lost_reason} maxLength={200} placeholder="Price, went with another tool, no reply…" />
        </Field>
      )}
      <Field id="owner_id" label="Owner">
        <Select id="owner_id" name="owner_id" defaultValue={lead.owner_id ?? ""}>
          <option value="">Unassigned</option>
          {owners.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </Select>
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field id="interest" label="Interested in">
          <Select id="interest" name="interest" defaultValue={lead.interest}>
            <option value="">Not known</option>
            {Object.entries(INTEREST_LABEL).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </Select>
        </Field>
        <Field id="plan_interest" label="Plan">
          <Select id="plan_interest" name="plan_interest" defaultValue={lead.plan_interest}>
            <option value="">Not known</option>
            {Object.entries(PLAN_LABEL).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </Select>
        </Field>
        <Field id="team_size" label="Team size"><Input id="team_size" name="team_size" defaultValue={lead.team_size} placeholder="e.g. 11-25" /></Field>
        <Field id="expected_value" label="Expected value (₹)"><Input id="expected_value" name="expected_value" inputMode="numeric" defaultValue={lead.expected_value || ""} placeholder="0" /></Field>
      </div>
      <Field id="notes" label="Pinned notes" hint="Kept at the top of this lead. Use Note in the timeline for dated entries.">
        <Textarea id="notes" name="notes" defaultValue={lead.notes} maxLength={10000} className="min-h-20" />
      </Field>
      <div className="flex items-center gap-3">
        <SubmitButton pending="Saving…">Save changes</SubmitButton>
        <SavedNote state={state} />
      </div>
    </form>
  );
}
