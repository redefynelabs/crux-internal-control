"use server";

import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";
import { api } from "@/lib/api";
import { failure, type FormState } from "@/lib/form";
import type { Sale } from "@/lib/types";

const str = (form: FormData, key: string) => String(form.get(key) ?? "").trim();
const amountOf = (form: FormData) => Number(str(form, "amount").replace(/[,\s₹]/g, "")) || 0;

function refresh(leadId?: string | null) {
  revalidatePath("/sales");
  revalidatePath("/analytics");
  revalidatePath("/leads");
  revalidatePath("/");
  if (leadId) revalidatePath(`/leads/${leadId}`);
}

function body(form: FormData) {
  return {
    customer: str(form, "customer"), company: str(form, "company"), email: str(form, "email"), plan: str(form, "plan"),
    billing: str(form, "billing"), amount: amountOf(form), started_on: str(form, "started_on") || undefined,
    owner_id: str(form, "owner_id"), notes: String(form.get("notes") ?? "").trim(),
  };
}

export async function createSale(_: FormState, form: FormData): Promise<FormState> {
  const b = body(form);
  if (!b.customer) return { error: "Enter the customer's name", fields: { customer: "Enter the customer's name" } };
  if (b.amount <= 0) return { error: "Enter the amount", fields: { amount: "Enter the amount" } };
  let sale: Sale;
  try {
    sale = await api<Sale>("/v1/sales", { method: "POST", body: { ...b, lead_id: str(form, "lead_id") || undefined } });
  } catch (e) {
    return failure(e);
  }
  refresh(sale.lead_id);
  redirect(`/sales/${sale.id}`);
}

export async function saveSale(id: string, _: FormState, form: FormData): Promise<FormState> {
  const b = body(form);
  if (!b.customer) return { error: "Enter the customer's name", fields: { customer: "Enter the customer's name" } };
  if (b.amount <= 0) return { error: "Enter the amount", fields: { amount: "Enter the amount" } };
  try {
    const sale = await api<Sale>(`/v1/sales/${encodeURIComponent(id)}`, { method: "PATCH", body: b });
    refresh(sale.lead_id);
  } catch (e) {
    return failure(e);
  }
  revalidatePath(`/sales/${id}`);
  return { saved: true };
}

export async function cancelSale(id: string, _: FormState, form: FormData): Promise<FormState> {
  try {
    const sale = await api<Sale>(`/v1/sales/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: { status: "cancelled", ended_on: str(form, "ended_on") || undefined, cancel_reason: str(form, "cancel_reason") },
    });
    refresh(sale.lead_id);
  } catch (e) {
    return failure(e);
  }
  revalidatePath(`/sales/${id}`);
  return { saved: true };
}

export async function reactivateSale(id: string) {
  const sale = await api<Sale>(`/v1/sales/${encodeURIComponent(id)}`, { method: "PATCH", body: { status: "active" } });
  refresh(sale.lead_id);
  revalidatePath(`/sales/${id}`);
}

export async function deleteSale(id: string) {
  await api(`/v1/sales/${encodeURIComponent(id)}`, { method: "DELETE" });
  refresh();
  redirect("/sales");
}
