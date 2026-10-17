"use client";

import { useActionState } from "react";
import { createLead } from "@/app/actions-leads";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input, Select } from "@/components/ui";
import { INTEREST_LABEL, PLAN_LABEL } from "@/lib/format";
import type { User } from "@/lib/types";

export function NewLeadForm({ users, meId }: { users: User[]; meId: string }) {
  const [state, action] = useActionState<FormState, FormData>(createLead, {});
  const f = state.fields ?? {};
  return (
    <form action={action} className="grid gap-4 sm:grid-cols-2">
      <Field id="name" label="Name" error={f.name}><Input id="name" name="name" required autoFocus /></Field>
      <Field id="email" label="Email" error={f.email}><Input id="email" name="email" type="email" required /></Field>
      <Field id="phone" label="Phone" error={f.phone}><Input id="phone" name="phone" type="tel" /></Field>
      <Field id="company" label="Company" error={f.company}><Input id="company" name="company" /></Field>
      <Field id="interest" label="Interested in">
        <Select id="interest" name="interest" defaultValue="">
          <option value="">Not known</option>
          {Object.entries(INTEREST_LABEL).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </Select>
      </Field>
      <Field id="plan_interest" label="Plan">
        <Select id="plan_interest" name="plan_interest" defaultValue="">
          <option value="">Not known</option>
          {Object.entries(PLAN_LABEL).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </Select>
      </Field>
      <Field id="team_size" label="Team size"><Input id="team_size" name="team_size" placeholder="e.g. 11-25" /></Field>
      <Field id="expected_value" label="Expected value (₹)"><Input id="expected_value" name="expected_value" inputMode="numeric" placeholder="0" /></Field>
      <Field id="owner_id" label="Owner">
        <Select id="owner_id" name="owner_id" defaultValue={meId}>
          <option value="">Unassigned</option>
          {users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </Select>
      </Field>
      <div className="flex items-center gap-3 sm:col-span-2">
        <SubmitButton pending="Adding…">Add lead</SubmitButton>
        <SavedNote state={state} />
      </div>
    </form>
  );
}
