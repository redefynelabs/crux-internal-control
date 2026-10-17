"use client";

import { useActionState } from "react";
import { changeStatus } from "@/app/actions-entitlements";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { ConfirmButton } from "@/components/confirm-button";
import { Card, Input } from "@/components/ui";
import type { FormState } from "@/lib/form";
import type { Installation } from "@/lib/types";

type Action = "suspend" | "resume" | "revoke";

function Form({ id, action, children }: { id: string; action: Action; children: (state: FormState) => React.ReactNode }) {
  const [state, run] = useActionState<FormState, FormData>(changeStatus.bind(null, id, action), {});
  return (
    <form action={run} className="space-y-2">
      {children(state)}
    </form>
  );
}

/** Stop or restore an installation without deleting anything: suspended is read-only, revoked gets nothing more. */
export function StatusForm({ installation }: { installation: Installation }) {
  const id = installation.id;
  const off = installation.status !== "active";
  return (
    <Card className="p-5">
      <h2 className="font-medium">Access</h2>
      <p className="mt-1 text-xs text-muted-foreground">Nothing is ever deleted. Data stays readable by the customer; only changes stop.</p>
      {off && (
        <p className="mt-3 rounded-xl bg-muted p-3 text-sm">
          <strong className="capitalize">{installation.status}</strong>{installation.status_note && `: ${installation.status_note}`}
        </p>
      )}
      {off ? (
        <div className="mt-3">
          <Form id={id} action="resume">
            {(s) => (
              <>
                <SubmitButton size="sm" pending="Resuming…">Resume access</SubmitButton>
                <SavedNote state={s} />
              </>
            )}
          </Form>
        </div>
      ) : (
        <div className="mt-3 space-y-4">
          <Form id={id} action="suspend">
            {(s) => (
              <Field id="suspend-note" label="Suspend (read-only until resumed)" error={s.fields?.note}>
                <Input id="suspend-note" name="note" maxLength={500} placeholder="Reason, shown to the customer" required />
                <div className="mt-2 flex items-center gap-3">
                  <ConfirmButton message="Suspend this customer? Their ERP becomes read-only at their next check." variant="secondary">Suspend</ConfirmButton>
                  <SavedNote state={s} />
                </div>
              </Field>
            )}
          </Form>
          <Form id={id} action="revoke">
            {(s) => (
              <Field id="revoke-note" label="Revoke the key" error={s.fields?.note}>
                <Input id="revoke-note" name="note" maxLength={500} placeholder="Reason, for the record" required />
                <div className="mt-2 flex items-center gap-3">
                  <ConfirmButton message="Revoke this key? The installation gets no more files or updates.">Revoke</ConfirmButton>
                  <SavedNote state={s} />
                </div>
              </Field>
            )}
          </Form>
        </div>
      )}
    </Card>
  );
}
