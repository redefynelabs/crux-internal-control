"use client";

import { Button, type ButtonVariant } from "./ui";

/** A button inside a form that asks first. */
export function ConfirmButton({ message, children, variant = "destructive" }: { message: string; children: React.ReactNode; variant?: ButtonVariant }) {
  return (
    <Button
      type="submit"
      variant={variant}
      size="sm"
      onClick={(e) => {
        if (!window.confirm(message)) e.preventDefault();
      }}
    >
      {children}
    </Button>
  );
}
