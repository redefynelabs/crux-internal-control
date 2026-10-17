import "server-only";
import { cache } from "react";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import type { AccessRequest, Analytics, AuditEntry, CatalogResponse, Demo, Installation, InstallationCredential, InstallationDetail, EmailSettings, GeneralSettings, Lead, LeadDetail, Release, Sale, Stats, Task, Template, User } from "./types";

export const SESSION_COOKIE = "cp_admin";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public fields: Record<string, string> = {},
  ) {
    super(message);
  }
}

function baseUrl(): string {
  return (process.env.CONTROL_PLANE_URL ?? "http://localhost:8090").replace(/\/$/, "");
}

/** The signed-in session token, or null. It lives in an httpOnly cookie, so browser scripts never see it. */
export async function sessionToken(): Promise<string | null> {
  return (await cookies()).get(SESSION_COOKIE)?.value ?? null;
}

/** Calls the control plane without a session (sign-in and accepting an invite). */
export async function publicRequest<T>(path: string, init: { method?: string; body?: unknown } = {}): Promise<T> {
  return request<T>(path, "", init);
}

/** Calls the control plane with a token. Every page and action goes through here, so the API's own check is the gate. */
export async function request<T>(path: string, token: string, init: { method?: string; body?: unknown } = {}): Promise<T> {
  let res: Response;
  try {
    res = await fetch(baseUrl() + path, {
      method: init.method ?? "GET",
      headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}), ...(init.body !== undefined ? { "Content-Type": "application/json" } : {}) },
      body: init.body !== undefined ? JSON.stringify(init.body) : undefined,
      cache: "no-store",
      signal: AbortSignal.timeout(10_000),
    });
  } catch {
    throw new ApiError(0, "unreachable", "The control plane cannot be reached");
  }
  if (res.status === 204) return undefined as T;
  const json = (await res.json().catch(() => ({}))) as { data?: T; error?: { code?: string; message?: string; fields?: Record<string, string> } };
  if (!res.ok) throw new ApiError(res.status, json.error?.code ?? "error", json.error?.message ?? "Request failed", json.error?.fields);
  return json.data as T;
}

/** As request, with the cookie's token. A missing or rejected token sends the person to sign in. */
export async function api<T>(path: string, init?: { method?: string; body?: unknown }): Promise<T> {
  const token = await sessionToken();
  if (!token) redirect("/login");
  try {
    return await request<T>(path, token, init);
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) redirect("/login");
    throw e;
  }
}


const qs = (params: Record<string, string | number | undefined>) => {
  const q = Object.entries(params).filter(([, v]) => v !== undefined && v !== "").map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`);
  return q.length ? `?${q.join("&")}` : "";
};

/** The signed-in person, fetched once per request. */
export const getMe = cache(() => api<User>("/v1/me"));
export const getStats = cache(() => api<Stats>("/v1/stats"));
export const getUsers = () => api<User[]>("/v1/users");
export interface LeadFilter {
  status?: string;
  owner?: string;
  interest?: string;
  due?: string;
  q?: string;
  limit?: number;
}
export const getLeads = (f: LeadFilter | string = {}, limit = 500) => {
  const filter: LeadFilter = typeof f === "string" ? { status: f, limit } : { limit, ...f };
  return api<Lead[]>(`/v1/leads${qs({ ...filter })}`);
};
export const getLead = (id: string) => api<LeadDetail>(`/v1/leads/${encodeURIComponent(id)}`);
export const getTasks = (assignee = "me", state = "open", limit = 200) => api<Task[]>(`/v1/tasks${qs({ assignee, state, limit })}`);
export const getTemplates = () => api<Template[]>("/v1/templates");
export const getEmailSettings = () => api<EmailSettings>("/v1/settings/email");
export const getGeneralSettings = () => api<GeneralSettings>("/v1/settings/general");
export const getDemos = (status?: string, limit = 500) => api<Demo[]>(`/v1/demos${qs({ status, limit })}`);
export interface SaleFilter {
  status?: string;
  billing?: string;
  q?: string;
}
export const getSales = (f: SaleFilter = {}) => api<Sale[]>(`/v1/sales${qs({ ...f, limit: 500 })}`);
export const getSale = (id: string) => api<Sale>(`/v1/sales/${encodeURIComponent(id)}`);
export const getAnalytics = (months = 12) => api<Analytics>(`/v1/analytics${qs({ months })}`);
export const getCatalog = () => api<CatalogResponse>("/v1/entitlements/catalog");
export const getInstallations = (kind?: string) => api<Installation[]>(`/v1/installations${qs({ kind })}`);
export const getReleases = () => api<Release[]>("/v1/releases");
export const getAudit = (installation?: string) => api<AuditEntry[]>(`/v1/audit${qs({ installation })}`);
export const getInstallation = (id: string) => api<InstallationDetail>(`/v1/installations/${encodeURIComponent(id)}`);
export const getInstallationCredential = (id: string) => api<InstallationCredential>(`/v1/installations/${encodeURIComponent(id)}/credential`);
export const getAccessRequests = (status?: string) => api<AccessRequest[]>(`/v1/access-requests${qs({ status })}`);
