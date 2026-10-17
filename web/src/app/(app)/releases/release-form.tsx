"use client";

import { useActionState } from "react";
import { publishRelease } from "@/app/actions-entitlements";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Card, Input, Select, Textarea } from "@/components/ui";
import type { FormState } from "@/lib/form";

/** Publishes a version. Images are by digest, so what customers pull is exactly what was published. */
export function ReleaseForm() {
  const [state, action] = useActionState<FormState, FormData>(publishRelease, {});
  const f = state.fields ?? {};
  return (
    <Card className="p-5">
      <h2 className="font-medium">Publish a release</h2>
      <p className="mt-1 text-xs text-muted-foreground">It is signed with your key. Each customer is offered it only if their maintenance covers its date.</p>
      <form action={action} className="mt-4 space-y-4">
        <div className="grid gap-4 sm:grid-cols-3">
          <Field id="version" label="Version" error={f.version} hint="Like 1.4.2">
            <Input id="version" name="version" placeholder="1.4.2" required maxLength={40} />
          </Field>
          <Field id="channel" label="Channel" error={f.channel} hint="Beta is never offered to customers.">
            <Select id="channel" name="channel" defaultValue="stable">
              <option value="stable">Stable</option>
              <option value="security">Security fix</option>
              <option value="beta">Beta</option>
            </Select>
          </Field>
          <Field id="published_on" label="Published on" hint="Today if empty. It decides who the AMC covers.">
            <Input id="published_on" name="published_on" type="date" />
          </Field>
        </div>
        <Field id="notes" label="What changed" error={f.notes} hint="Customers read this before updating.">
          <Textarea id="notes" name="notes" maxLength={8000} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field id="image_server" label="Server image" error={f.images} hint="registry/erp-server@sha256:…">
            <Input id="image_server" name="image_server" required placeholder="registry.example.com/erp-server@sha256:…" />
          </Field>
          <Field id="image_web" label="Web image">
            <Input id="image_web" name="image_web" required placeholder="registry.example.com/erp-web@sha256:…" />
          </Field>
          <Field id="image_demo_tools" label="Demo tools image (optional)">
            <Input id="image_demo_tools" name="image_demo_tools" placeholder="registry.example.com/erp-demo-tools@sha256:…" />
          </Field>
          <Field id="min_from_version" label="Oldest version it upgrades from" error={f.min_from_version} hint="Leave empty if any version can go straight to it.">
            <Input id="min_from_version" name="min_from_version" placeholder="1.2.0" maxLength={40} />
          </Field>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" name="has_migrations" className="size-4" /> Changes the database (customers should back up first)
        </label>
        <div className="flex items-center gap-3">
          <SubmitButton pending="Publishing…">Sign and publish</SubmitButton>
          <SavedNote state={state} />
        </div>
      </form>
    </Card>
  );
}
