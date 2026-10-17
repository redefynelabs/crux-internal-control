"use client";

import { useActionState, useRef, useState, useEffect } from "react";
import { createTask } from "@/app/actions-leads";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { PlusIcon } from "@/components/icons";
import { SubmitButton } from "@/components/pending-button";
import { Button, Card, Input, Select } from "@/components/ui";
import { toLocalInput } from "@/lib/format";
import type { Lead, User } from "@/lib/types";

/** Adds a follow-up for any lead without opening it first. */
export function NewTask({ leads, users, meId }: { leads: Pick<Lead, "id" | "name" | "company">[]; users: User[]; meId: string }) {
  const [open, setOpen] = useState(false);
  const [state, action] = useActionState<FormState, FormData>(createTask, {});
  const ref = useRef<HTMLFormElement>(null);
  const [local, setLocal] = useState(() => {
    const d = new Date(Date.now() + 24 * 3600_000);
    d.setHours(10, 0, 0, 0);
    return toLocalInput(d);
  });
  const iso = local ? new Date(local).toISOString() : "";
  const f = state.fields ?? {};

  useEffect(() => {
    if (state.saved) {
      ref.current?.reset();
      // eslint-disable-next-line react-hooks/set-state-in-effect -- closing the form after a successful save
      setOpen(false);
    }
  }, [state]);

  if (!open) return <Button onClick={() => setOpen(true)}><PlusIcon className="size-4" /> New task</Button>;
  return (
    <Card className="mb-4 w-full p-5">
      <form ref={ref} action={action} className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <input type="hidden" name="due_at" value={iso} />
        <Field id="t_lead" label="Lead" error={f.lead_id}>
          <Select id="t_lead" name="lead_id" defaultValue="" required>
            <option value="" disabled>Choose a lead</option>
            {leads.map((l) => <option key={l.id} value={l.id}>{l.name}{l.company ? ` · ${l.company}` : ""}</option>)}
          </Select>
        </Field>
        <Field id="t_title" label="What needs doing" error={f.title}><Input id="t_title" name="title" required maxLength={200} placeholder="Call to discuss pricing" /></Field>
        <Field id="t_kind" label="Type">
          <Select id="t_kind" name="kind" defaultValue="call"><option value="call">Call</option><option value="email">Email</option><option value="other">Other</option></Select>
        </Field>
        <Field id="t_due" label="Due" error={f.due_at}><Input id="t_due" type="datetime-local" value={local} onChange={(e) => setLocal(e.target.value)} required /></Field>
        <Field id="t_assignee" label="Assigned to">
          <Select id="t_assignee" name="assignee_id" defaultValue={meId}>{users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}</Select>
        </Field>
        <div className="flex items-end gap-3 sm:col-span-2 lg:col-span-3">
          <SubmitButton pending="Adding…">Add task</SubmitButton>
          <Button type="button" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
          <SavedNote state={state} />
        </div>
      </form>
    </Card>
  );
}
