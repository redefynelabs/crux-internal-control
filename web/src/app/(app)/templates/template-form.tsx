"use client";

import { useActionState, useEffect, useState, useTransition } from "react";
import { previewTemplate, saveTemplate, sendTemplateTest } from "@/app/actions-admin";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Button, Input, Select, Textarea } from "@/components/ui";
import type { Template, TemplatePreview } from "@/lib/types";

const SHORTCODES: [string, string][] = [
  ["[[button: Open your demo | {{demo_url}}]]", "A button. Left out when its link is empty."],
  ["[[steps: Do this | Then this | Then that]]", "A numbered list."],
  ["[[details: Email = {{login_email}} | Password = {{login_password}}]]", "A box of label and value rows."],
];

export function TemplateForm({ template }: { template: Template }) {
  const [state, action] = useActionState<FormState, FormData>(saveTemplate.bind(null, template.id), {});
  const id = `t-${template.id}`;
  const [format, setFormat] = useState<"text" | "html">(template.format ?? "text");
  const [subject, setSubject] = useState(template.subject);
  const [body, setBody] = useState(template.body);
  const [preview, setPreview] = useState<TemplatePreview | null>(null);
  const [previewError, setPreviewError] = useState("");
  const [view, setView] = useState<"email" | "text">("email");
  const [testing, startTest] = useTransition();
  const [testNote, setTestNote] = useState("");

  // Show the message as it will look, a moment after the person stops typing.
  useEffect(() => {
    let live = true;
    const t = setTimeout(async () => {
      const out = await previewTemplate(format, subject, body);
      if (!live) return;
      setPreview(out.preview ?? null);
      setPreviewError(out.error ?? "");
    }, 450);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [format, subject, body]);

  const designed = format === "html";
  return (
    <form action={action} className="space-y-4">
      <input type="hidden" name="format" value={format} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field id={`${id}-name`} label="Name"><Input id={`${id}-name`} name="name" defaultValue={template.name} maxLength={100} required /></Field>
        <Field id={`${id}-subject`} label="Subject"><Input id={`${id}-subject`} name="subject" value={subject} onChange={(e) => setSubject(e.target.value)} maxLength={200} required /></Field>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <div className="space-y-4">
          <Field id={`${id}-format`} label="Format" hint={designed ? "Write the message only. The Crux logo, card and footer are added when it is sent, with a plain-text copy for mail apps that need one." : "Sent exactly as typed, as plain text."}>
            <Select id={`${id}-format`} value={format} onChange={(e) => setFormat(e.target.value === "html" ? "html" : "text")}>
              <option value="html">Designed email (HTML, with the logo)</option>
              <option value="text">Plain text</option>
            </Select>
          </Field>
          <Field id={`${id}-body`} label={designed ? "Message (HTML)" : "Message"}>
            <Textarea id={`${id}-body`} name="body" value={body} onChange={(e) => setBody(e.target.value)} className="min-h-72 font-mono text-xs" maxLength={20000} required spellCheck={false} />
          </Field>
          {designed && (
            <div className="rounded-lg border bg-muted/40 p-3 text-xs text-muted-foreground">
              <p className="mb-2 font-medium text-foreground">Ready-made blocks</p>
              <ul className="space-y-1.5">
                {SHORTCODES.map(([code, what]) => (
                  <li key={code}><code className="break-all font-mono text-[11px] text-foreground">{code}</code><br />{what}</li>
                ))}
              </ul>
              <p className="mt-2">Plain HTML works too: <code className="font-mono text-[11px]">&lt;h1&gt;</code>, <code className="font-mono text-[11px]">&lt;p&gt;</code>, <code className="font-mono text-[11px]">&lt;ul&gt;</code>, <code className="font-mono text-[11px]">&lt;a&gt;</code>, <code className="font-mono text-[11px]">&lt;strong&gt;</code>.</p>
            </div>
          )}
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <p className="text-sm font-medium">Preview <span className="font-normal text-muted-foreground">(sample values)</span></p>
            {designed && (
              <div role="tablist" aria-label="Preview as" className="flex gap-1 text-xs">
                {(["email", "text"] as const).map((v) => (
                  <button key={v} type="button" role="tab" aria-selected={view === v} onClick={() => setView(v)} className={`rounded-full px-2.5 py-1 ${view === v ? "bg-foreground text-background" : "bg-muted text-muted-foreground"}`}>{v === "email" ? "Email" : "Plain text"}</button>
                ))}
              </div>
            )}
          </div>
          {previewError ? (
            <p role="alert" className="text-sm text-destructive">{previewError}</p>
          ) : (
            <div className="overflow-hidden rounded-lg border bg-[#f3f5fa]">
              <p className="truncate border-b bg-background px-3 py-2 text-xs"><span className="text-muted-foreground">Subject: </span>{preview?.subject ?? ""}</p>
              {designed && view === "email" ? (
                <iframe title={`Preview of ${template.name}`} sandbox="" srcDoc={preview?.html ?? ""} className="block h-[560px] w-full bg-[#f3f5fa]" />
              ) : (
                <pre className="h-[560px] overflow-auto whitespace-pre-wrap bg-background p-4 text-xs leading-relaxed">{preview?.text ?? ""}</pre>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <SubmitButton pending="Saving…">Save template</SubmitButton>
        <Button
          type="button"
          variant="secondary"
          size="sm"
          disabled={testing}
          onClick={() =>
            startTest(async () => {
              setTestNote("");
              const out = await sendTemplateTest(format, subject, body);
              setTestNote(out.sentTo ? `Sent to ${out.sentTo}` : out.error ?? "Could not send");
            })
          }
        >
          {testing ? "Sending…" : "Send a test to me"}
        </Button>
        {testNote && <span role="status" className="text-sm text-muted-foreground">{testNote}</span>}
        <SavedNote state={state} />
      </div>
    </form>
  );
}
