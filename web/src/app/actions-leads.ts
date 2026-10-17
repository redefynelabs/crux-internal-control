"use server";

import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";
import { ApiError, api } from "@/lib/api";
import { LEAD_STATUSES, type Lead } from "@/lib/types";
import { failure, type FormState } from "@/lib/form";

const str = (form: FormData, key: string) => String(form.get(key) ?? "").trim();

function refresh(id?: string) {
  if (id) revalidatePath(`/leads/${id}`);
  revalidatePath("/leads");
  revalidatePath("/tasks");
  revalidatePath("/");
}

export async function createLead(_: FormState, form: FormData): Promise<FormState> {
  const value = Number(str(form, "expected_value").replace(/[,\s]/g, "")) || 0;
  if (!str(form, "name")) return { error: "Enter a name", fields: { name: "Enter a name" } };
  if (!str(form, "email")) return { error: "Enter an email address", fields: { email: "Enter an email address" } };
  let lead: Lead;
  try {
    lead = await api<Lead>("/v1/leads", {
      method: "POST",
      body: {
        name: str(form, "name"), email: str(form, "email"), phone: str(form, "phone"), company: str(form, "company"),
        interest: str(form, "interest"), plan_interest: str(form, "plan_interest"), team_size: str(form, "team_size"),
        expected_value: value, owner_id: str(form, "owner_id"),
      },
    });
  } catch (e) {
    return failure(e);
  }
  refresh();
  redirect(`/leads/${lead.id}`);
}

export async function saveLead(id: string, _: FormState, form: FormData): Promise<FormState> {
  const status = str(form, "status");
  if (!(LEAD_STATUSES as readonly string[]).includes(status)) return { error: "Choose a status" };
  const body: Record<string, unknown> = {
    status,
    owner_id: str(form, "owner_id"),
    interest: str(form, "interest"),
    plan_interest: str(form, "plan_interest"),
    team_size: str(form, "team_size"),
    expected_value: Number(str(form, "expected_value").replace(/[,\s]/g, "")) || 0,
    lost_reason: status === "lost" ? str(form, "lost_reason") : "",
  };
  if (form.has("notes")) body.notes = String(form.get("notes") ?? "");
  try {
    await api(`/v1/leads/${encodeURIComponent(id)}`, { method: "PATCH", body });
  } catch (e) {
    return failure(e);
  }
  refresh(id);
  return { saved: true };
}

export async function addNote(id: string, _: FormState, form: FormData): Promise<FormState> {
  const text = str(form, "text");
  if (!text) return { error: "Write a note first" };
  try {
    await api(`/v1/leads/${encodeURIComponent(id)}/notes`, { method: "POST", body: { text } });
  } catch (e) {
    return failure(e);
  }
  refresh(id);
  return { saved: true };
}

export async function logCall(id: string, _: FormState, form: FormData): Promise<FormState> {
  try {
    await api(`/v1/leads/${encodeURIComponent(id)}/calls`, { method: "POST", body: { outcome: str(form, "outcome") || "reached", text: str(form, "text") } });
  } catch (e) {
    return failure(e);
  }
  refresh(id);
  return { saved: true };
}

export async function sendEmail(id: string, _: FormState, form: FormData): Promise<FormState> {
  const subject = str(form, "subject");
  const body = String(form.get("body") ?? "").trim();
  if (!subject) return { error: "Add a subject", fields: { subject: "Add a subject" } };
  if (!body) return { error: "Write the message", fields: { body: "Write the message" } };
  try {
    await api(`/v1/leads/${encodeURIComponent(id)}/emails`, { method: "POST", body: { template_id: str(form, "template_id") || undefined, subject, body } });
  } catch (e) {
    if (e instanceof ApiError && e.code === "email_not_configured") return { error: "email_not_configured" };
    return failure(e);
  }
  refresh(id);
  return { saved: true };
}

export async function addTask(id: string, _: FormState, form: FormData): Promise<FormState> {
  const title = str(form, "title");
  const due = str(form, "due_at"); // an ISO instant, made in the browser from the local time
  if (!title) return { error: "Name the task", fields: { title: "Name the task" } };
  if (!due || Number.isNaN(Date.parse(due))) return { error: "Pick when it is due", fields: { due_at: "Pick a date and time" } };
  try {
    await api(`/v1/leads/${encodeURIComponent(id)}/tasks`, {
      method: "POST",
      body: { title, kind: str(form, "kind") || "other", due_at: due, assignee_id: str(form, "assignee_id") || undefined },
    });
  } catch (e) {
    return failure(e);
  }
  refresh(id);
  return { saved: true };
}

export async function setTaskDone(id: string, leadId: string | undefined, done: boolean) {
  await api(`/v1/tasks/${encodeURIComponent(id)}`, { method: "PATCH", body: { done } });
  refresh(leadId);
}

export async function deleteTask(id: string, leadId: string | undefined) {
  await api(`/v1/tasks/${encodeURIComponent(id)}`, { method: "DELETE" });
  refresh(leadId);
}

/** Moves a lead to another status from the board. */
export async function moveLead(id: string, status: string, lostReason?: string): Promise<{ error?: string }> {
  if (!(LEAD_STATUSES as readonly string[]).includes(status)) return { error: "Unknown status" };
  try {
    await api(`/v1/leads/${encodeURIComponent(id)}`, { method: "PATCH", body: { status, ...(status === "lost" ? { lost_reason: lostReason ?? "" } : {}) } });
  } catch (e) {
    const f = failure(e);
    return { error: f.error ?? "Could not move the lead" };
  }
  refresh(id);
  return {};
}

/** Changes when a task is due (dragging it to another day on the board). */
export async function rescheduleTask(id: string, dueAt: string): Promise<{ error?: string }> {
  if (Number.isNaN(Date.parse(dueAt))) return { error: "Pick a valid time" };
  try {
    await api(`/v1/tasks/${encodeURIComponent(id)}`, { method: "PATCH", body: { due_at: dueAt, done: false } });
  } catch (e) {
    return { error: failure(e).error ?? "Could not move the task" };
  }
  refresh();
  return {};
}

export async function completeTask(id: string, done: boolean): Promise<{ error?: string }> {
  try {
    await api(`/v1/tasks/${encodeURIComponent(id)}`, { method: "PATCH", body: { done } });
  } catch (e) {
    return { error: failure(e).error ?? "Could not update the task" };
  }
  refresh();
  return {};
}

/** Adds a task for any lead, from the tasks page. */
export async function createTask(_: FormState, form: FormData): Promise<FormState> {
  const leadId = str(form, "lead_id");
  if (!leadId) return { error: "Choose a lead", fields: { lead_id: "Choose a lead" } };
  return addTask(leadId, _, form);
}
