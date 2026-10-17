"use client";

import { useActionState } from "react";
import { cancelSale } from "@/app/actions-sales";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input } from "@/components/ui";

export function CancelForm({ id, recurring }: { id: string; recurring: boolean }) {
  const [state, action] = useActionState<FormState, FormData>(cancelSale.bind(null, id), {});
  return (
    <form action={action} className="mt-3 space-y-3">
      <p className="text-sm text-muted-foreground">{recurring ? "Its revenue leaves MRR from the date below." : "A refunded or reversed sale leaves the numbers."}</p>
      <Field id="ended_on" label="Ends on"><Input id="ended_on" name="ended_on" type="date" defaultValue={new Date().toLocaleDateString("en-CA")} /></Field>
      <Field id="cancel_reason" label="Why?"><Input id="cancel_reason" name="cancel_reason" maxLength={200} placeholder="Price, switched tool, closed down…" /></Field>
      <div className="flex items-center gap-3"><SubmitButton variant="destructive" size="sm" pending="Cancelling…">Cancel sale</SubmitButton><SavedNote state={state} /></div>
    </form>
  );
}
