"use client";

import { useActionState } from "react";
import { inviteUser, type InviteState } from "@/app/actions-admin";
import { Field } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input, Select } from "@/components/ui";
import { CopyLink } from "./copy-link";

export function InviteForm() {
  const [state, action] = useActionState<InviteState, FormData>(inviteUser, {});
  const f = state.fields ?? {};
  return (
    <>
      <form action={action} className="space-y-4">
        <Field id="name" label="Name" error={f.name}><Input id="name" name="name" required /></Field>
        <Field id="email" label="Email" error={f.email}><Input id="email" name="email" type="email" required /></Field>
        <Field id="role" label="Role" hint="Sales: leads, tasks, email, demos. Admin: also team, templates and settings.">
          <Select id="role" name="role" defaultValue="sales">
            <option value="sales">Sales</option>
            <option value="admin">Admin</option>
          </Select>
        </Field>
        {state.error && <p role="alert" className="text-sm text-destructive">{state.error}</p>}
        <SubmitButton pending="Inviting…" className="w-full">Send invite</SubmitButton>
      </form>
      {state.inviteUrl && (
        <div role="status" className="mt-4 space-y-2 border-t pt-4 text-sm">
          <p>{state.emailSent ? `An invite email was sent to ${state.invitedEmail}.` : "Email is not set up, so no email went out. Send this link yourself:"}</p>
          <CopyLink path={state.inviteUrl} />
        </div>
      )}
    </>
  );
}
