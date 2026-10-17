"use client";

import { useActionState } from "react";
import { createSale, saveSale } from "@/app/actions-sales";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input, Select, Textarea } from "@/components/ui";
import { BILLING_LABEL, SALE_PLAN_LABEL } from "@/lib/format";
import type { Sale, User } from "@/lib/types";

/** Records a sale, or edits one when `sale` is given. `prefill` carries what is known from the lead being closed. */
export function SaleForm({ sale, prefill, users, meId }: { sale?: Sale; prefill?: Partial<Sale>; users: User[]; meId: string }) {
  const init = sale ?? prefill ?? {};
  const [state, action] = useActionState<FormState, FormData>(sale ? saveSale.bind(null, sale.id) : createSale, {});
  const f = state.fields ?? {};
  const owners = users.filter((u) => u.active || u.id === init.owner_id);
  return (
    <form action={action} className="grid gap-4 sm:grid-cols-2">
      {!sale && prefill?.lead_id && <input type="hidden" name="lead_id" value={prefill.lead_id} />}
      <Field id="customer" label="Customer" error={f.customer}><Input id="customer" name="customer" defaultValue={init.customer} required autoFocus={!sale} /></Field>
      <Field id="company" label="Company" error={f.company}><Input id="company" name="company" defaultValue={init.company} /></Field>
      <Field id="email" label="Email" error={f.email}><Input id="email" name="email" type="email" defaultValue={init.email} /></Field>
      <Field id="plan" label="Plan" error={f.plan}>
        <Select id="plan" name="plan" defaultValue={init.plan ?? ""}>
          <option value="">Not set</option>
          {Object.entries(SALE_PLAN_LABEL).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </Select>
      </Field>
      <Field id="billing" label="Billed" error={f.billing}>
        <Select id="billing" name="billing" defaultValue={init.billing ?? "monthly"}>
          {Object.entries(BILLING_LABEL).map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </Select>
      </Field>
      <Field id="amount" label="Amount (₹)" error={f.amount} hint="What the customer pays each time they are billed, or once.">
        <Input id="amount" name="amount" inputMode="numeric" defaultValue={init.amount ? String(init.amount) : ""} required />
      </Field>
      <Field id="started_on" label="Starts on" error={f.started_on}><Input id="started_on" name="started_on" type="date" defaultValue={init.started_on ?? new Date().toLocaleDateString("en-CA")} required /></Field>
      <Field id="owner_id" label="Sold by">
        <Select id="owner_id" name="owner_id" defaultValue={init.owner_id ?? meId}>
          <option value="">Nobody</option>
          {owners.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </Select>
      </Field>
      <div className="sm:col-span-2">
        <Field id="notes" label="Notes"><Textarea id="notes" name="notes" defaultValue={init.notes} className="min-h-20" placeholder="Contract terms, discount, who to bill…" /></Field>
      </div>
      <div className="flex items-center gap-3 sm:col-span-2">
        <SubmitButton pending="Saving…">{sale ? "Save changes" : "Record sale"}</SubmitButton>
        <SavedNote state={state} />
      </div>
    </form>
  );
}
