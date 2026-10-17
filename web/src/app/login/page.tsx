import type { Metadata } from "next";
import { LoginForm } from "./login-form";

export const metadata: Metadata = { title: "Sign in" };

export default function LoginPage() {
  return (
    <div className="grid min-h-screen place-items-center px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="grid size-11 place-items-center rounded-2xl bg-primary text-lg font-bold text-primary-foreground">C</span>
          <h1 className="mt-4 text-2xl font-semibold tracking-tight">Control plane</h1>
          <p className="mt-1 text-sm text-muted-foreground">Sign in to follow up leads and manage demos.</p>
        </div>
        <LoginForm />
      </div>
    </div>
  );
}
