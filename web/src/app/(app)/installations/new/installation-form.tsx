"use client";

import { useActionState } from "react";
import { createInstallation } from "@/app/actions-entitlements";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input, Textarea } from "@/components/ui";

export function InstallationForm() {
  const [state, action] = useActionState<FormState, FormData>(createInstallation, {});
  const f = state.fields ?? {};
  return (
    <form action={action} className="grid gap-4">
      <Field id="name" label="Customer" error={f.name}><Input id="name" name="name" required autoFocus /></Field>
      <Field id="notes" label="Notes" error={f.notes}><Textarea id="notes" name="notes" className="min-h-20" placeholder="Contract, plan, who to contact…" /></Field>
      <div className="flex items-center gap-3">
        <SubmitButton pending="Adding…">Add customer</SubmitButton>
        <SavedNote state={state} />
      </div>
    </form>
  );
}
