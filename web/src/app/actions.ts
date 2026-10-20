"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";
import { ApiError, api, publicRequest, request, SESSION_COOKIE, sessionToken } from "@/lib/api";
import { failure, type FormState } from "@/lib/form";
import type { User } from "@/lib/types";

async function startSession(token: string) {
  (await cookies()).set(SESSION_COOKIE, token, {
    httpOnly: true,
    sameSite: "strict",
    // Plain HTTP is allowed only when asked for (a private network); the token never travels unprotected by default.
    secure: process.env.ADMIN_INSECURE_COOKIE !== "true",
    path: "/",
    maxAge: 60 * 60 * 12,
  });
}

export async function login(_: FormState, form: FormData): Promise<FormState> {
  const email = String(form.get("email") ?? "").trim();
  const password = String(form.get("password") ?? "");
  if (!email || !password) return { error: "Enter your email and password" };
  try {
    const out = await publicRequest<{ token: string; user: User }>("/v1/auth/login", { method: "POST", body: { email, password } });
    await startSession(out.token);
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) return { error: "That email and password do not match" };
    return failure(e);
  }
  redirect("/");
}

export async function acceptInvite(token: string, _: FormState, form: FormData): Promise<FormState> {
  const password = String(form.get("password") ?? "");
  const again = String(form.get("again") ?? "");
  const name = String(form.get("name") ?? "").trim();
  if (password.length < 10) return { error: "Use at least 10 characters", fields: { password: "At least 10 characters" } };
  if (password !== again) return { error: "The two passwords do not match", fields: { again: "Does not match" } };
  try {
    const out = await publicRequest<{ token: string; user: User }>("/v1/auth/accept-invite", { method: "POST", body: { token, password, ...(name ? { name } : {}) } });
    await startSession(out.token);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return { error: "This invite link is no longer valid. Ask an admin for a new one" };
    return failure(e);
  }
  redirect("/");
}

export async function logout() {
  const token = await sessionToken();
  if (token) await request("/v1/auth/logout", token, { method: "POST" }).catch(() => {});
  (await cookies()).delete(SESSION_COOKIE);
  redirect("/login");
}

export async function extendDemo(id: string, form: FormData) {
  const minutes = Number(form.get("minutes"));
  await api(`/v1/demos/${encodeURIComponent(id)}/extend`, { method: "POST", body: { minutes } });
  revalidatePath("/demos");
  revalidatePath("/leads/[id]", "page");
  revalidatePath("/");
}

/** Deletes a lead with its history, tasks, emails and demos (admins only). A lead with a sale or a customer installation is refused. */
export async function deleteLead(id: string) {
  try {
    await api(`/v1/leads/${encodeURIComponent(id)}`, { method: "DELETE" });
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) redirect(`/leads/${encodeURIComponent(id)}?blocked=1`);
    throw e;
  }
  revalidatePath("/leads");
  revalidatePath("/demos");
  revalidatePath("/");
  redirect("/leads");
}

export async function endDemo(id: string) {
  await api(`/v1/demos/${encodeURIComponent(id)}`, { method: "DELETE" });
  revalidatePath("/demos");
  revalidatePath("/leads/[id]", "page");
  revalidatePath("/");
}
