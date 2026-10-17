"use client";

import { useActionState, useState } from "react";
import { saveGeneralSettings } from "@/app/actions-admin";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Input, Select } from "@/components/ui";
import type { GeneralSettings } from "@/lib/types";

function Toggle({ name, label, text, checked, onChange }: { name: string; label: string; text: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-start gap-3 text-sm">
      <input type="checkbox" name={name} checked={checked} onChange={(e) => onChange(e.target.checked)} className="mt-0.5 size-4 rounded border-input" />
      <span><span className="font-medium">{label}</span><span className="block text-muted-foreground">{text}</span></span>
    </label>
  );
}

export function GeneralForm({ settings }: { settings: GeneralSettings }) {
  const [state, action] = useActionState<FormState, FormData>(saveGeneralSettings, {});
  const [confirm, setConfirm] = useState(settings.auto_confirm);
  const [followup, setFollowup] = useState(settings.auto_demo_followup);
  return (
    <form action={action} className="space-y-5">
      <div>
        <h2 className="font-medium">New leads</h2>
        <div className="mt-3 grid gap-4 sm:grid-cols-2">
          <Field id="assignment" label="Who gets a new lead?" hint="Round robin shares new leads between active team members in turn.">
            <Select id="assignment" name="assignment" defaultValue={settings.assignment}>
              <option value="unassigned">Nobody. Someone picks it up</option>
              <option value="round_robin">Round robin</option>
            </Select>
          </Field>
          <Field id="followup_hours" label="First follow-up due after (hours)" hint="A task is created for the owner when a lead arrives.">
            <Input id="followup_hours" name="followup_hours" type="number" min={1} max={720} defaultValue={settings.followup_hours} />
          </Field>
        </div>
      </div>
      <div className="space-y-4 border-t pt-5">
        <h2 className="font-medium">Automatic emails</h2>
        <Toggle name="auto_confirm" label="Confirm contact and cloud requests" text="Send the “thanks for getting in touch” template as soon as someone writes in." checked={confirm} onChange={setConfirm} />
        <Toggle name="auto_demo_followup" label="Follow up after a demo" text="Send the demo follow-up template after a visitor's demo ends." checked={followup} onChange={setFollowup} />
        {followup && (
          <div className="max-w-xs">
            <Field id="demo_followup_delay_minutes" label="Send after the demo ends (minutes)">
              <Input id="demo_followup_delay_minutes" name="demo_followup_delay_minutes" type="number" min={0} max={10080} defaultValue={settings.demo_followup_delay_minutes} />
            </Field>
          </div>
        )}
        <p className="text-xs text-muted-foreground">Automatic emails use the templates and need the email settings to be saved and enabled.</p>
      </div>
      {/* Switched-off options still post their numbers so the saved values are kept. */}
      {!followup && <input type="hidden" name="demo_followup_delay_minutes" value={settings.demo_followup_delay_minutes} />}
      <div className="flex items-center gap-3"><SubmitButton pending="Saving…">Save settings</SubmitButton><SavedNote state={state} /></div>
    </form>
  );
}
