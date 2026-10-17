"use server";

import { revalidatePath } from "next/cache";
import { api } from "@/lib/api";
import type { EmailSettings, GeneralSettings, InviteResult } from "@/lib/types";
import { failure, type FormState } from "@/lib/form";

const str = (form: FormData, key: string) => String(form.get(key) ?? "").trim();

export interface InviteState extends FormState {
  inviteUrl?: string;
  emailSent?: boolean;
  invitedEmail?: string;
}

export async function inviteUser(_: InviteState, form: FormData): Promise<InviteState> {
  const name = str(form, "name");
  const email = str(form, "email");
  if (!name) return { error: "Enter a name", fields: { name: "Enter a name" } };
  if (!email) return { error: "Enter an email address", fields: { email: "Enter an email address" } };
  try {
    const out = await api<InviteResult>("/v1/users", { method: "POST", body: { name, email, role: str(form, "role") || "sales" } });
    revalidatePath("/team");
    return { saved: true, inviteUrl: out.invite_url, emailSent: out.email_sent, invitedEmail: out.user.email };
  } catch (e) {
    return failure(e);
  }
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars -- the form action passes (previous state, form data)
export async function reinviteUser(id: string, _: InviteState, __?: FormData): Promise<InviteState> {
  try {
    const out = await api<InviteResult>(`/v1/users/${encodeURIComponent(id)}/invite`, { method: "POST" });
    revalidatePath("/team");
    return { saved: true, inviteUrl: out.invite_url, emailSent: out.email_sent, invitedEmail: out.user.email };
  } catch (e) {
    return failure(e);
  }
}

export async function updateUser(id: string, patch: { role?: string; active?: boolean }): Promise<FormState> {
  try {
    await api(`/v1/users/${encodeURIComponent(id)}`, { method: "PATCH", body: patch });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/team");
  return { saved: true };
}

export async function saveTemplate(id: string, _: FormState, form: FormData): Promise<FormState> {
  const name = str(form, "name");
  const subject = str(form, "subject");
  const body = String(form.get("body") ?? "").trim();
  if (!name || !subject || !body) return { error: "Name, subject and message are all needed" };
  try {
    await api(`/v1/templates/${encodeURIComponent(id)}`, { method: "PUT", body: { name, subject, body } });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/templates");
  return { saved: true };
}

export async function saveEmailSettings(_: FormState, form: FormData): Promise<FormState> {
  const body: Partial<EmailSettings> & { password?: string } = {
    enabled: form.get("enabled") === "on",
    host: str(form, "host"),
    port: Number(str(form, "port")) || 587,
    security: (str(form, "security") || "starttls") as EmailSettings["security"],
    username: str(form, "username"),
    from_name: str(form, "from_name"),
    from_email: str(form, "from_email"),
    reply_to: str(form, "reply_to"),
    signature: String(form.get("signature") ?? "").trim(),
  };
  const password = String(form.get("password") ?? "");
  if (password) body.password = password;
  try {
    await api("/v1/settings/email", { method: "PUT", body });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/settings");
  return { saved: true };
}

export async function sendTestEmail(_: FormState, form: FormData): Promise<FormState> {
  const to = str(form, "to");
  if (!to) return { error: "Enter an address to send the test to" };
  try {
    await api("/v1/settings/email/test", { method: "POST", body: { to } });
  } catch (e) {
    return failure(e);
  }
  return { saved: true, info: `Test email sent to ${to}. Check the inbox (and spam).` };
}

export async function saveGeneralSettings(_: FormState, form: FormData): Promise<FormState> {
  const body: GeneralSettings = {
    assignment: str(form, "assignment") === "round_robin" ? "round_robin" : "unassigned",
    auto_confirm: form.get("auto_confirm") === "on",
    auto_demo_followup: form.get("auto_demo_followup") === "on",
    followup_hours: Math.max(1, Number(str(form, "followup_hours")) || 24),
    demo_followup_delay_minutes: Math.max(0, Number(str(form, "demo_followup_delay_minutes")) || 60),
  };
  try {
    await api("/v1/settings/general", { method: "PUT", body });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/settings");
  return { saved: true };
}

export async function changePassword(_: FormState, form: FormData): Promise<FormState> {
  const current = String(form.get("current") ?? "");
  const next = String(form.get("next") ?? "");
  const again = String(form.get("again") ?? "");
  if (next.length < 10) return { error: "Use at least 10 characters", fields: { next: "At least 10 characters" } };
  if (next !== again) return { error: "The two passwords do not match", fields: { again: "Does not match" } };
  try {
    await api("/v1/me/password", { method: "POST", body: { current, new: next } });
  } catch (e) {
    return failure(e);
  }
  return { saved: true };
}
