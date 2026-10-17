"use client";

import { useActionState } from "react";
import { acceptInvite } from "@/app/actions";
import type { FormState } from "@/lib/form";
import { SubmitButton } from "@/components/pending-button";
import { Card, Input, Label } from "@/components/ui";

export function InviteForm({ token, name }: { token: string; name: string }) {
  const [state, action] = useActionState<FormState, FormData>(acceptInvite.bind(null, token), {});
  const f = state.fields ?? {};
  return (
    <Card className="p-6">
      <form action={action} className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="name">Your name</Label>
          <Input id="name" name="name" defaultValue={name} autoComplete="name" />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="password">Password</Label>
          <Input id="password" name="password" type="password" autoComplete="new-password" minLength={10} required aria-invalid={f.password ? true : undefined} />
          <p className="text-xs text-muted-foreground">At least 10 characters.</p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="again">Password again</Label>
          <Input id="again" name="again" type="password" autoComplete="new-password" required aria-invalid={f.again ? true : undefined} />
        </div>
        {state.error && <p role="alert" className="text-sm text-destructive">{state.error}</p>}
        <SubmitButton pending="Creating…" className="w-full" size="lg">Create my account</SubmitButton>
      </form>
    </Card>
  );
}
