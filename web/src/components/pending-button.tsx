"use client";

import { useFormStatus } from "react-dom";
import { Button, type ButtonSize, type ButtonVariant } from "./ui";

export function SubmitButton({ children, pending: label, variant, size, className }: { className?: string; children: React.ReactNode; pending?: string; variant?: ButtonVariant; size?: ButtonSize }) {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" variant={variant} size={size} className={className} disabled={pending}>
      {pending ? (label ?? "Working…") : children}
    </Button>
  );
}
