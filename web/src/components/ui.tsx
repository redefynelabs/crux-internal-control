import type { ComponentProps } from "react";
import Link from "next/link";
import { cn } from "@/lib/utils";

// Pill buttons, rounded cards with a hairline ring and soft inputs: the ERP's component shapes.

const BUTTON = {
  base: "inline-flex shrink-0 items-center justify-center gap-1.5 rounded-4xl border border-transparent text-sm font-medium whitespace-nowrap transition-all outline-none select-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:translate-y-px disabled:pointer-events-none disabled:opacity-50",
  variant: {
    default: "bg-primary text-primary-foreground hover:bg-primary/85",
    outline: "border-border bg-input/30 hover:bg-input/50",
    secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/70",
    ghost: "hover:bg-muted",
    destructive: "bg-destructive/10 text-destructive hover:bg-destructive/20",
  },
  size: { default: "h-9 px-4", sm: "h-8 px-3", lg: "h-10 px-5" },
};

export type ButtonVariant = keyof typeof BUTTON.variant;
export type ButtonSize = keyof typeof BUTTON.size;

export function buttonClass(variant: ButtonVariant = "default", size: ButtonSize = "default", className?: string) {
  return cn(BUTTON.base, BUTTON.variant[variant], BUTTON.size[size], className);
}

export function Button({ variant, size, className, ...props }: ComponentProps<"button"> & { variant?: ButtonVariant; size?: ButtonSize }) {
  return <button className={buttonClass(variant, size, className)} {...props} />;
}

export function ButtonLink({ variant, size, className, ...props }: ComponentProps<typeof Link> & { variant?: ButtonVariant; size?: ButtonSize }) {
  return <Link className={buttonClass(variant, size, className)} {...props} />;
}

export function Card({ className, ...props }: ComponentProps<"div">) {
  return <div className={cn("rounded-2xl bg-card text-card-foreground ring-1 ring-foreground/10", className)} {...props} />;
}

export function Badge({ className, ...props }: ComponentProps<"span">) {
  return <span className={cn("inline-flex h-5 w-fit items-center gap-1 rounded-4xl px-2 text-xs font-medium whitespace-nowrap", className)} {...props} />;
}

const FIELD = "w-full min-w-0 border border-input bg-input/30 text-sm transition-colors outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:opacity-50";

export function Input({ className, ...props }: ComponentProps<"input">) {
  return <input className={cn(FIELD, "h-9 rounded-4xl px-3.5", className)} {...props} />;
}

export function Select({ className, ...props }: ComponentProps<"select">) {
  return <select className={cn(FIELD, "h-9 rounded-4xl px-3 pr-8", className)} {...props} />;
}

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return <textarea className={cn(FIELD, "min-h-28 rounded-xl px-3.5 py-2.5", className)} {...props} />;
}

export function Label({ className, ...props }: ComponentProps<"label">) {
  return <label className={cn("text-sm font-medium", className)} {...props} />;
}

export function PageHeader({ title, description, actions }: { title: string; description?: string; actions?: React.ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions}
    </div>
  );
}

export function EmptyState({ title, text }: { title: string; text?: string }) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-14 text-center">
      <p className="font-medium">{title}</p>
      {text && <p className="mt-1 max-w-sm text-sm text-muted-foreground">{text}</p>}
    </div>
  );
}
