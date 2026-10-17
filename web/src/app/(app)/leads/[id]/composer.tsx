"use client";

import Link from "next/link";
import { useActionState, useEffect, useRef, useState } from "react";
import { addNote, addTask, logCall, sendEmail } from "@/app/actions-leads";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { MailIcon, NoteIcon, PhoneIcon, TasksIcon } from "@/components/icons";
import { SubmitButton } from "@/components/pending-button";
import { Input, Select, Textarea } from "@/components/ui";
import { toLocalInput } from "@/lib/format";
import type { Lead, Template, User } from "@/lib/types";
import { cn } from "@/lib/utils";

const TABS = [
  { id: "note", label: "Note", icon: NoteIcon },
  { id: "email", label: "Email", icon: MailIcon },
  { id: "call", label: "Log call", icon: PhoneIcon },
  { id: "task", label: "Add task", icon: TasksIcon },
] as const;
type Tab = (typeof TABS)[number]["id"];

/** Fills the placeholders the person can see now; {{signature}} is added by the server when the email is sent. */
function fill(text: string, vars: Record<string, string>) {
  return text.replace(/\{\{(\w+)\}\}/g, (m, k: string) => vars[k] ?? m);
}

/** Clears a form after it saved. */
function useResetOnSave(state: FormState, ref: React.RefObject<HTMLFormElement | null>) {
  useEffect(() => {
    if (state.saved) ref.current?.reset();
  }, [state, ref]);
}

function NoteForm({ leadId }: { leadId: string }) {
  const [state, action] = useActionState<FormState, FormData>(addNote.bind(null, leadId), {});
  const ref = useRef<HTMLFormElement>(null);
  useResetOnSave(state, ref);
  return (
    <form ref={ref} action={action} className="space-y-3">
      <Textarea name="text" aria-label="Note" placeholder="What happened, what they need, what to do next…" className="min-h-24" maxLength={5000} required />
      <div className="flex items-center gap-3"><SubmitButton pending="Saving…">Add note</SubmitButton><SavedNote state={state} /></div>
    </form>
  );
}

function CallForm({ leadId }: { leadId: string }) {
  const [state, action] = useActionState<FormState, FormData>(logCall.bind(null, leadId), {});
  const ref = useRef<HTMLFormElement>(null);
  useResetOnSave(state, ref);
  return (
    <form ref={ref} action={action} className="space-y-3">
      <Field id="outcome" label="Outcome">
        <Select id="outcome" name="outcome" defaultValue="reached">
          <option value="reached">Spoke to them</option>
          <option value="no_answer">No answer</option>
          <option value="voicemail">Left a voicemail</option>
          <option value="wrong_number">Wrong number</option>
        </Select>
      </Field>
      <Textarea name="text" aria-label="What was said" placeholder="What was said (optional)" className="min-h-20" maxLength={5000} />
      <div className="flex items-center gap-3"><SubmitButton pending="Saving…">Log call</SubmitButton><SavedNote state={state} /></div>
    </form>
  );
}

function EmailForm({ lead, me, templates, demoUrl, isAdmin }: { lead: Lead; me: User; templates: Template[]; demoUrl: string; isAdmin: boolean }) {
  const [templateId, setTemplateId] = useState("");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [state, action] = useActionState<FormState, FormData>(async (prev, form) => {
    const out = await sendEmail(lead.id, prev, form);
    if (out.saved) {
      setTemplateId("");
      setSubject("");
      setBody("");
    }
    return out;
  }, {});
  const f = state.fields ?? {};
  const vars = { name: lead.name, first_name: lead.name.split(/\s+/)[0] ?? lead.name, company: lead.company, owner: lead.owner_name || me.name, demo_url: demoUrl };
  const pick = (id: string) => {
    setTemplateId(id);
    const t = templates.find((x) => x.id === id);
    if (t) {
      setSubject(fill(t.subject, vars));
      setBody(fill(t.body, vars));
    }
  };
  return (
    <form action={action} className="space-y-3">
      <input type="hidden" name="template_id" value={templateId} />
      <p className="text-sm text-muted-foreground">To <span className="text-foreground">{lead.email}</span>. Replies go to your inbox.</p>
      <Field id="template" label="Start from a template">
        <Select id="template" value={templateId} onChange={(e) => pick(e.target.value)}>
          <option value="">Blank</option>
          {templates.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </Select>
      </Field>
      <Field id="subject" label="Subject" error={f.subject}><Input id="subject" name="subject" value={subject} onChange={(e) => setSubject(e.target.value)} maxLength={200} required /></Field>
      <Field id="body" label="Message" error={f.body} hint="Your signature is added at the end.">
        <Textarea id="body" name="body" value={body} onChange={(e) => setBody(e.target.value)} className="min-h-44" maxLength={20000} required />
      </Field>
      <div className="flex flex-wrap items-center gap-3">
        <SubmitButton pending="Sending…">Send email</SubmitButton>
        {state.error === "email_not_configured" ? (
          <p role="alert" className="text-sm text-destructive">
            Email is not set up yet. {isAdmin ? <Link href="/settings" className="underline">Open email settings</Link> : "Ask an admin to set it up."}
          </p>
        ) : (
          <SavedNote state={state.saved ? { saved: true, info: "Email queued. It appears in the timeline." } : state} />
        )}
      </div>
    </form>
  );
}

function TaskForm({ leadId, users, meId }: { leadId: string; users: User[]; meId: string }) {
  const [state, action] = useActionState<FormState, FormData>(addTask.bind(null, leadId), {});
  const ref = useRef<HTMLFormElement>(null);
  // Tomorrow at 10:00 in the person's own time zone. This form only mounts after a click, so it is never server rendered.
  const [local, setLocal] = useState(() => {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    d.setHours(10, 0, 0, 0);
    return toLocalInput(d);
  });
  const f = state.fields ?? {};
  useEffect(() => {
    if (state.saved) ref.current?.reset();
  }, [state]);
  const iso = local && !Number.isNaN(Date.parse(local)) ? new Date(local).toISOString() : "";
  return (
    <form ref={ref} action={action} className="space-y-3">
      <input type="hidden" name="due_at" value={iso} />
      <Field id="title" label="What needs doing?" error={f.title}><Input id="title" name="title" placeholder="Call to discuss the Business plan" maxLength={200} required /></Field>
      <div className="grid gap-3 sm:grid-cols-3">
        <Field id="kind" label="Type">
          <Select id="kind" name="kind" defaultValue="call">
            <option value="call">Call</option>
            <option value="email">Email</option>
            <option value="other">Other</option>
          </Select>
        </Field>
        <Field id="due_local" label="Due" error={f.due_at}><Input id="due_local" type="datetime-local" value={local} onChange={(e) => setLocal(e.target.value)} required /></Field>
        <Field id="assignee_id" label="Assigned to">
          <Select id="assignee_id" name="assignee_id" defaultValue={meId}>
            {users.filter((u) => u.active).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
          </Select>
        </Field>
      </div>
      <div className="flex items-center gap-3"><SubmitButton pending="Adding…">Add task</SubmitButton><SavedNote state={state} /></div>
    </form>
  );
}

export function Composer({ lead, me, users, templates, demoUrl }: { lead: Lead; me: User; users: User[]; templates: Template[]; demoUrl: string }) {
  const [tab, setTab] = useState<Tab>("note");
  return (
    <div>
      <div role="tablist" aria-label="Add to this lead" className="mb-4 flex flex-wrap gap-1.5">
        {TABS.map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            role="tab"
            type="button"
            id={`tab-${id}`}
            aria-selected={tab === id}
            aria-controls={`panel-${id}`}
            onClick={() => setTab(id)}
            className={cn("inline-flex h-8 items-center gap-1.5 rounded-4xl px-3 text-sm font-medium transition-colors", tab === id ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}
          >
            <Icon className="size-4" /> {label}
          </button>
        ))}
      </div>
      <div role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}>
        {tab === "note" && <NoteForm leadId={lead.id} />}
        {tab === "email" && <EmailForm lead={lead} me={me} templates={templates} demoUrl={demoUrl} isAdmin={me.role === "admin"} />}
        {tab === "call" && <CallForm leadId={lead.id} />}
        {tab === "task" && <TaskForm leadId={lead.id} users={users} meId={me.id} />}
      </div>
    </div>
  );
}
