"use server";

import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";
import { api } from "@/lib/api";
import { failure, type ActivationState, type FormState } from "@/lib/form";
import type { Installation } from "@/lib/types";

// A date means the end of that day, so the customer has all of it.
const endOfDay = (ymd: string) => (ymd ? new Date(`${ymd}T23:59:59Z`).toISOString() : null);

const str = (form: FormData, key: string) => String(form.get(key) ?? "").trim();

export async function createInstallation(_: FormState, form: FormData): Promise<FormState> {
  const name = str(form, "name");
  if (!name) return { error: "Enter the customer's name", fields: { name: "Enter the customer's name" } };
  let inst: Installation;
  try {
    inst = await api<Installation>("/v1/installations", { method: "POST", body: { name, notes: String(form.get("notes") ?? "").trim(), lead_id: str(form, "lead_id") } });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/installations");
  redirect(`/installations/${inst.id}`);
}

/** Signs the next entitlement from the editor. Only words that differ from the defaults are sent. */
export async function issueEntitlement(id: string, defaults: Record<string, { singular: string; plural: string }>, _: FormState, form: FormData): Promise<FormState> {
  const labels: Record<string, { singular: string; plural: string }> = {};
  for (const [key, d] of Object.entries(defaults)) {
    const singular = str(form, `label_${key}_singular`);
    const plural = str(form, `label_${key}_plural`);
    if (singular && plural && (singular !== d.singular || plural !== d.plural)) labels[key] = { singular, plural };
  }
  const license = str(form, "license");
  try {
    await api(`/v1/installations/${encodeURIComponent(id)}/entitlements`, {
      method: "POST",
      body: {
        template: str(form, "template"),
        apps: form.getAll("apps").map(String),
        features: form.getAll("features").map(String),
        labels,
        license,
        // A lifetime licence has no end date, whatever is left in the field.
        expires_at: license === "lifetime" ? null : endOfDay(str(form, "expires_on")),
        updates_until: endOfDay(str(form, "updates_on")),
        approve_requests: form.getAll("approve").map(String),
      },
    });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/installations");
  revalidatePath(`/installations/${id}`);
  redirect(`/installations/${id}`);
}

export async function declineRequest(installationId: string, requestId: string, _: FormState, form: FormData): Promise<FormState> {
  const note = str(form, "note");
  if (!note) return { error: "Write a short note the customer will see", fields: { note: "Write a short note" } };
  try {
    await api(`/v1/installations/${encodeURIComponent(installationId)}/requests/${encodeURIComponent(requestId)}/decline`, { method: "POST", body: { note } });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/installations");
  revalidatePath(`/installations/${installationId}`);
  return { saved: true };
}

export async function rotateCredential(id: string) {
  await api(`/v1/installations/${encodeURIComponent(id)}/credential/rotate`, { method: "POST" });
  revalidatePath(`/installations/${id}`);
}

/** Makes a one-time activation code. It is shown once, from this form's state. */
export async function makeActivationCode(id: string): Promise<ActivationState> {
  try {
    const out = await api<{ code: string; expires_at: string }>(`/v1/installations/${encodeURIComponent(id)}/activation`, { method: "POST" });
    revalidatePath(`/installations/${id}`);
    return { saved: true, code: out.code, expires: out.expires_at };
  } catch (e) {
    return failure(e);
  }
}

/** Changes the licence terms (kind, subscription end, maintenance end) and signs a new file with the rest unchanged. */
export async function saveLicence(id: string, _: FormState, form: FormData): Promise<FormState> {
  const license = str(form, "license");
  try {
    await api(`/v1/installations/${encodeURIComponent(id)}/licence`, {
      method: "POST",
      body: {
        license,
        expires_at: license === "lifetime" ? null : endOfDay(str(form, "expires_on")),
        updates_until: endOfDay(str(form, "updates_on")),
        note: str(form, "note"),
      },
    });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/installations");
  revalidatePath(`/installations/${id}`);
  return { saved: true, info: "Licence saved and a new file signed" };
}

/** Suspend, resume or revoke. A reason is required for the first and last. */
export async function changeStatus(id: string, action: "suspend" | "resume" | "revoke", _: FormState, form: FormData): Promise<FormState> {
  const note = str(form, "note");
  if (action !== "resume" && !note) return { error: "Write a short reason", fields: { note: "Write a short reason" } };
  try {
    await api(`/v1/installations/${encodeURIComponent(id)}/${action}`, { method: "POST", body: { note } });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/installations");
  revalidatePath(`/installations/${id}`);
  return { saved: true, info: action === "resume" ? "Resumed" : action === "suspend" ? "Suspended" : "Revoked" };
}

/** Publishes a release: version, channel, notes, images by digest. */
export async function publishRelease(_: FormState, form: FormData): Promise<FormState> {
  const images: Record<string, string> = {};
  for (const key of ["server", "web", "demo_tools"]) {
    const v = str(form, `image_${key}`);
    if (v) images[key] = v;
  }
  const date = str(form, "published_on");
  try {
    await api("/v1/releases", {
      method: "POST",
      body: {
        version: str(form, "version"),
        channel: str(form, "channel"),
        published_at: date ? new Date(`${date}T00:00:00Z`).toISOString() : null,
        notes: String(form.get("notes") ?? "").trim(),
        has_migrations: form.get("has_migrations") === "on",
        min_from_version: str(form, "min_from_version"),
        images,
      },
    });
  } catch (e) {
    return failure(e);
  }
  revalidatePath("/releases");
  return { saved: true, info: "Release published" };
}

export async function withdrawRelease(id: string) {
  await api(`/v1/releases/${encodeURIComponent(id)}/withdraw`, { method: "POST" });
  revalidatePath("/releases");
}
