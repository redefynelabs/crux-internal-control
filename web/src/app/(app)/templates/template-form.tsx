"use client";

import { useActionState } from "react";
import { saveTemplate } from "@/app/actions-admin";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input, Textarea } from "@/components/ui";
import type { Template } from "@/lib/types";

export function TemplateForm({ template }: { template: Template }) {
  const [state, action] = useActionState<FormState, FormData>(saveTemplate.bind(null, template.id), {});
  const id = `t-${template.id}`;
  return (
    <form action={action} className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field id={`${id}-name`} label="Name"><Input id={`${id}-name`} name="name" defaultValue={template.name} maxLength={100} required /></Field>
        <Field id={`${id}-subject`} label="Subject"><Input id={`${id}-subject`} name="subject" defaultValue={template.subject} maxLength={200} required /></Field>
      </div>
      <Field id={`${id}-body`} label="Message">
        <Textarea id={`${id}-body`} name="body" defaultValue={template.body} className="min-h-40 font-mono text-xs" maxLength={20000} required />
      </Field>
      <div className="flex items-center gap-3"><SubmitButton pending="Saving…">Save template</SubmitButton><SavedNote state={state} /></div>
    </form>
  );
}
