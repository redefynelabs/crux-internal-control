"use client";

import { useActionState } from "react";
import { login } from "@/app/actions";
import type { FormState } from "@/lib/form";
import { SubmitButton } from "@/components/pending-button";
import { Card, Input, Label } from "@/components/ui";

export function LoginForm() {
  const [state, action] = useActionState<FormState, FormData>(login, {});
  return (
    <Card className="p-6">
      <form action={action} className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="email">Email</Label>
          <Input id="email" name="email" type="email" autoComplete="username" autoFocus required />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="password">Password</Label>
          <Input id="password" name="password" type="password" autoComplete="current-password" required aria-invalid={state.error ? true : undefined} aria-describedby={state.error ? "login-error" : undefined} />
          {state.error && <p id="login-error" role="alert" className="text-sm text-destructive">{state.error}</p>}
        </div>
        <SubmitButton pending="Signing in…" className="w-full" size="lg">Sign in</SubmitButton>
      </form>
    </Card>
  );
}
