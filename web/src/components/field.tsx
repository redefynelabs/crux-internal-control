import type { ReactNode } from "react";
import { Label } from "./ui";

/** A label, a control and its error. */
export function Field({ id, label, error, hint, children }: { id: string; label: string; error?: string; hint?: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && !error && <p className="text-xs text-muted-foreground">{hint}</p>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    </div>
  );
}

export function SavedNote({ state }: { state: { saved?: boolean; error?: string; info?: string } }) {
  return (
    <>
      {state.saved && <p role="status" className="text-sm text-emerald-600 dark:text-emerald-400">{state.info ?? "Saved"}</p>}
      {state.error && <p role="alert" className="text-sm text-destructive">{state.error}</p>}
    </>
  );
}
