"use client";

import { useActionState, useState } from "react";
import { saveEmailSettings, sendTestEmail } from "@/app/actions-admin";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input, Select, Textarea } from "@/components/ui";
import type { EmailSettings } from "@/lib/types";

const DEFAULT_PORT = { starttls: 587, tls: 465, none: 25 } as const;

export function EmailForm({ settings }: { settings: EmailSettings }) {
  const [state, action] = useActionState<FormState, FormData>(saveEmailSettings, {});
  const [security, setSecurity] = useState(settings.security);
  const [port, setPort] = useState(String(settings.port || DEFAULT_PORT[settings.security]));
  const f = state.fields ?? {};
  return (
    <form action={action} className="space-y-4">
      <label className="flex items-start gap-3 text-sm">
        <input type="checkbox" name="enabled" defaultChecked={settings.enabled} className="mt-0.5 size-4 rounded border-input" />
        <span><span className="font-medium">Send email from the control plane</span><span className="block text-muted-foreground">Turn off to pause all outgoing email, including automatic messages.</span></span>
      </label>
      <div className="grid gap-4 sm:grid-cols-3">
        <div className="sm:col-span-2"><Field id="host" label="Server" error={f.host}><Input id="host" name="host" defaultValue={settings.host} placeholder="smtp.example.com" autoComplete="off" /></Field></div>
        <Field id="port" label="Port" error={f.port}><Input id="port" name="port" inputMode="numeric" value={port} onChange={(e) => setPort(e.target.value)} /></Field>
      </div>
      <Field id="security" label="Security">
        <Select
          id="security"
          name="security"
          value={security}
          onChange={(e) => {
            const next = e.target.value as EmailSettings["security"];
            // Move the port with the mode only while it is still one of the usual ones.
            if (Object.values(DEFAULT_PORT).map(String).includes(port)) setPort(String(DEFAULT_PORT[next]));
            setSecurity(next);
          }}
        >
          <option value="starttls">STARTTLS (usually port 587)</option>
          <option value="tls">TLS (usually port 465)</option>
          <option value="none">None (only for a trusted local relay)</option>
        </Select>
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field id="username" label="Username" error={f.username}><Input id="username" name="username" defaultValue={settings.username} autoComplete="off" /></Field>
        <Field id="password" label="Password" error={f.password} hint={settings.has_password ? "A password is saved. Leave empty to keep it." : undefined}>
          <Input id="password" name="password" type="password" autoComplete="new-password" placeholder={settings.has_password ? "••••••••" : ""} />
        </Field>
        <Field id="from_name" label="From name" error={f.from_name}><Input id="from_name" name="from_name" defaultValue={settings.from_name} placeholder="Redefyne Sales" /></Field>
        <Field id="from_email" label="From address" error={f.from_email}><Input id="from_email" name="from_email" type="email" defaultValue={settings.from_email} placeholder="sales@example.com" /></Field>
        <div className="sm:col-span-2"><Field id="reply_to" label="Reply-to (optional)" error={f.reply_to} hint="Replies go here. Leave empty to use the sender's own address."><Input id="reply_to" name="reply_to" type="email" defaultValue={settings.reply_to} /></Field></div>
      </div>
      <Field id="signature" label="Signature" hint="Added to the end of every email.">
        <Textarea id="signature" name="signature" defaultValue={settings.signature} className="min-h-24" maxLength={2000} />
      </Field>
      <div className="flex items-center gap-3"><SubmitButton pending="Saving…">Save email settings</SubmitButton><SavedNote state={state} /></div>
    </form>
  );
}

export function TestEmailForm({ defaultTo }: { defaultTo: string }) {
  const [state, action] = useActionState<FormState, FormData>(sendTestEmail, {});
  return (
    <form action={action} className="space-y-3">
      <Field id="to" label="Send to"><Input id="to" name="to" type="email" defaultValue={defaultTo} required /></Field>
      <SubmitButton variant="outline" pending="Sending…">Send test email</SubmitButton>
      <SavedNote state={state} />
    </form>
  );
}
