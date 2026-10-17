"use client";

import { useActionState, useState } from "react";
import { saveLicence } from "@/app/actions-entitlements";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Card, Input, Select } from "@/components/ui";
import type { FormState } from "@/lib/form";
import type { EntitlementPayload } from "@/lib/types";

/** Licence terms: subscription or lifetime, when a subscription ends, and until when new releases are included (the AMC). */
export function LicenceForm({ installationId, current }: { installationId: string; current?: EntitlementPayload }) {
  const [state, action] = useActionState<FormState, FormData>(saveLicence.bind(null, installationId), {});
  const [type, setType] = useState(current?.license?.type ?? (current?.expires_at ? "subscription" : "lifetime"));
  const f = state.fields ?? {};
  return (
    <Card className="p-5">
      <h2 className="font-medium">Licence and maintenance</h2>
      <p className="mt-1 text-xs text-muted-foreground">Saving signs a new file with the apps and features unchanged.</p>
      <form action={action} className="mt-3 space-y-3">
        <Field id="licence-type" label="Licence" error={f.license}>
          <Select id="licence-type" name="license" value={type} onChange={(e) => setType(e.target.value as "subscription" | "lifetime")}>
            <option value="subscription">Subscription (ends on a date)</option>
            <option value="lifetime">Lifetime (never ends)</option>
          </Select>
        </Field>
        {type === "subscription" && (
          <Field id="licence-expires" label="Subscription ends on" error={f.expires_at} hint="After this date the ERP is read-only following 14 days.">
            <Input id="licence-expires" name="expires_on" type="date" defaultValue={current?.expires_at?.slice(0, 10) ?? ""} required />
          </Field>
        )}
        <Field id="licence-updates" label="Updates included until (AMC)" error={f.updates_until} hint="Releases published after this date are not offered. The software keeps running either way.">
          <Input id="licence-updates" name="updates_on" type="date" defaultValue={current?.updates_until?.slice(0, 10) ?? ""} />
        </Field>
        <Field id="licence-note" label="Note (for the activity log)">
          <Input id="licence-note" name="note" maxLength={200} placeholder="Renewed for one year, invoice 1042" />
        </Field>
        <div className="flex items-center gap-3">
          <SubmitButton size="sm" pending="Signing…">Save licence</SubmitButton>
          <SavedNote state={state} />
        </div>
      </form>
    </Card>
  );
}
