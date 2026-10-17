"use client";

import { useActionState, useEffect, useRef } from "react";
import { changePassword } from "@/app/actions-admin";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input } from "@/components/ui";

export function AccountForm() {
  const [state, action] = useActionState<FormState, FormData>(changePassword, {});
  const ref = useRef<HTMLFormElement>(null);
  useEffect(() => {
    if (state.saved) ref.current?.reset();
  }, [state]);
  const f = state.fields ?? {};
  return (
    <form ref={ref} action={action} className="space-y-4">
      <Field id="current" label="Current password"><Input id="current" name="current" type="password" autoComplete="current-password" required /></Field>
      <Field id="next" label="New password" error={f.next} hint="At least 10 characters."><Input id="next" name="next" type="password" autoComplete="new-password" minLength={10} required /></Field>
      <Field id="again" label="New password again" error={f.again}><Input id="again" name="again" type="password" autoComplete="new-password" required /></Field>
      <div className="flex items-center gap-3"><SubmitButton pending="Saving…">Change password</SubmitButton><SavedNote state={state.saved ? { saved: true, info: "Password changed" } : { error: state.error }} /></div>
    </form>
  );
}
