"use client";

import { useActionState } from "react";
import { declineRequest } from "@/app/actions-entitlements";
import type { FormState } from "@/lib/form";
import { SubmitButton } from "@/components/pending-button";
import { Input } from "@/components/ui";

export function DeclineForm({ installationId, requestId }: { installationId: string; requestId: string }) {
  const [state, action] = useActionState<FormState, FormData>(declineRequest.bind(null, installationId, requestId), {});
  return (
    <form action={action} className="flex flex-wrap items-center gap-2">
      <Input name="note" aria-label="Note the customer will see" placeholder="Note for the customer" className="h-8 w-56" required maxLength={500} />
      <SubmitButton variant="secondary" size="sm" pending="Declining…">Decline</SubmitButton>
      {state.error && <p role="alert" className="text-sm text-destructive">{state.error}</p>}
    </form>
  );
}
