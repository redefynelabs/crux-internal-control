// The control plane's JSON, as returned by its back-office API.

export const LEAD_STATUSES = ["new", "demo_active", "demo_ended", "contacted", "qualified", "proposal", "won", "lost"] as const;
export type LeadStatus = (typeof LEAD_STATUSES)[number];

export const INTERESTS = ["cloud", "on_prem", "other"] as const;
export type Interest = (typeof INTERESTS)[number];
export const PLANS = ["start", "grow", "complete", "small", "business", "larger"] as const;
export type Plan = (typeof PLANS)[number];

export type Role = "admin" | "sales";

export interface User {
  id: string;
  name: string;
  email: string;
  role: Role;
  active: boolean;
  pending_invite: boolean;
  last_login_at: string | null;
  created_at: string;
}

export interface Lead {
  id: string;
  name: string;
  email: string;
  phone: string;
  company: string;
  source: string;
  status: LeadStatus;
  notes: string;
  interest: "" | Interest;
  plan_interest: "" | Plan;
  team_size: string;
  expected_value: number;
  lost_reason: string;
  owner_id: string | null;
  owner_name: string;
  last_contact_at: string | null;
  next_follow_up_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface LeadEvent {
  kind: string;
  detail: Record<string, unknown> | null;
  at: string;
  actor_name: string;
}

export type TaskKind = "call" | "email" | "other";

export interface Task {
  id: string;
  lead_id: string;
  lead_name: string;
  title: string;
  kind: TaskKind;
  due_at: string;
  assignee_id: string | null;
  assignee_name: string;
  done_at: string | null;
  created_at: string;
}

export interface SentEmail {
  id: string;
  lead_id: string;
  subject: string;
  body: string;
  to_email: string;
  status: "queued" | "sent" | "failed";
  error: string;
  sent_by_name: string;
  created_at: string;
  sent_at: string | null;
}

export interface LeadDetail {
  sale: Sale | null;
  lead: Lead;
  events: LeadEvent[];
  tasks: Task[];
  emails: SentEmail[];
}

export interface Template {
  id: string;
  key: string;
  name: string;
  subject: string;
  body: string;
  /** "text" is plain text; "html" holds the message content, which the Crux layout (logo, card, footer) wraps when it is sent. */
  format: "text" | "html";
  updated_at: string;
}

export interface TemplatePreview {
  subject: string;
  html: string;
  text: string;
}

export interface EmailSettings {
  enabled: boolean;
  host: string;
  port: number;
  security: "starttls" | "tls" | "none";
  username: string;
  has_password: boolean;
  from_name: string;
  from_email: string;
  reply_to: string;
  signature: string;
}

export interface GeneralSettings {
  assignment: "unassigned" | "round_robin";
  auto_confirm: boolean;
  auto_demo_followup: boolean;
  followup_hours: number;
  demo_followup_delay_minutes: number;
}

export interface InviteResult {
  user: User;
  invite_url: string;
  email_sent: boolean;
}

export type DemoStatus = "queued" | "provisioning" | "active" | "failed" | "expired" | "abandoned";

export interface Demo {
  id: string;
  lead_id: string;
  slug: string;
  status: DemoStatus;
  url: string;
  ip?: string;
  error?: string;
  created_at: string;
  expires_at: string;
  ended_at?: string | null;
  /** When the visitor joined the waiting line; absent for a demo that started at once. */
  queued_at?: string | null;
}

export interface Stats {
  leads: Partial<Record<LeadStatus, number>>;
  demos: Partial<Record<DemoStatus, number>>;
  unassigned: number;
  overdue_tasks: number;
  my_open_tasks: number;
  my_overdue_tasks: number;
  emails_failed: number;
  new_this_week: number;
  won_this_month: number;
  won_value_this_month: number;
  pipeline_value: number;
}

export const BILLINGS = ["monthly", "yearly", "one_time"] as const;
export type Billing = (typeof BILLINGS)[number];
export const SALE_PLANS = ["start", "grow", "complete", "small", "business", "larger", "custom"] as const;

export interface Sale {
  id: string;
  lead_id: string | null;
  customer: string;
  company: string;
  email: string;
  plan: string;
  billing: Billing;
  amount: number;
  mrr: number;
  status: "active" | "cancelled";
  started_on: string;
  ended_on: string | null;
  cancel_reason: string;
  owner_id: string | null;
  owner_name: string;
  notes: string;
  created_at: string;
}

export interface MonthPoint {
  month: string;
  mrr: number;
  new_mrr: number;
  churned_mrr: number;
  customers: number;
  one_time: number;
  new_leads: number;
  won: number;
  demos: number;
}

export interface Analytics {
  mrr: number;
  arr: number;
  customers: number;
  arpa: number;
  new_mrr_30: number;
  churned_mrr_30: number;
  churn_rate_90: number;
  one_time_12m: number;
  pipeline_value: number;
  weighted_pipeline: number;
  open_leads: number;
  win_rate: number;
  avg_deal: number;
  avg_days_to_close: number;
  tasks_done_30: number;
  tasks_on_time_30: number;
  series: MonthPoint[];
  funnel: { key: string; label: string; count: number }[];
  sources: { source: string; leads: number; won: number }[];
  plans: { plan: string; mrr: number; customers: number }[];
  owners: { id: string; name: string; leads: number; won: number; sold_mrr: number; open_tasks: number; overdue_tasks: number; done_tasks_30: number }[];
}

export interface EntitlementFeature {
  name: string;
  label: string;
  /** Set when the feature is sold in fewer editions than its app. */
  editions?: Edition[];
}
/** A business outcome the vendor ticks; it stands for apps and features. */
export interface EntitlementGroup {
  key: string;
  label: string;
  text: string;
  apps: string[];
  features: string[];
}
/** The two products the ERP is sold as; an installation has one. */
export type Edition = "services" | "goods";
export interface EntitlementCatalog {
  core: EntitlementFeature[];
  editions: { key: Edition; label: string; text: string }[];
  groups: EntitlementGroup[];
  apps: { name: string; label: string; features: EntitlementFeature[]; needs?: string[]; editions: Edition[] }[];
  templates: { id: string; name: string; summary: string; edition: Edition; apps: string[]; features: string[]; labels: Record<string, { singular: string; plural: string }> }[];
  records: { key: string; singular: string; plural: string }[];
}
export interface CatalogResponse {
  catalog: EntitlementCatalog;
  can_issue: boolean;
  public_key?: string;
  /** Where customers fetch the one-line installer; absent until installs are set up on this control plane. */
  install_url?: string;
}

export type InstallationState = "demo" | "revoked" | "suspended" | "awaiting_activation" | "expired" | "unreachable" | "out_of_date" | "active";
export type LicenseType = "subscription" | "lifetime" | "";

export interface Installation {
  id: string;
  name: string;
  kind: "customer" | "demo";
  demo_id: string | null;
  lead_id: string | null;
  lead_name: string;
  notes: string;
  created_at: string;
  /** The newest file issued; version 0 means none yet. */
  version: number;
  template: string;
  /** The newest file's edition; empty for one issued before editions. */
  edition: Edition | "";
  expires_at: string | null;
  issued_at: string | null;
  pending_requests: number;
  app_count: number;
  feature_count: number;
  license_type: LicenseType;
  /** The end of the maintenance contract (AMC): releases published after it are not offered. */
  updates_until: string | null;
  status: "active" | "suspended" | "revoked";
  status_note: string;
  activated_at: string | null;
  last_seen_at: string | null;
  applied_version: number | null;
  server_version: string;
  server_build: string;
  /** Worked out by the control plane, never typed in. */
  state: InstallationState;
  state_text: string;
  amc: "none" | "ok" | "ending" | "lapsed";
}

export interface Activation {
  id: string;
  hint: string;
  expires_at: string;
  used_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface AuditEntry {
  id: number;
  at: string;
  actor_email: string;
  installation_id: string | null;
  installation_name: string;
  action: string;
  detail: Record<string, unknown>;
}

export interface Release {
  id: string;
  version: string;
  channel: "stable" | "beta" | "security";
  published_at: string;
  notes: string;
  has_migrations: boolean;
  min_from_version: string;
  images: Record<string, string>;
  withdrawn_at: string | null;
  created_at: string;
  running: number;
}

export interface EntitlementPayload {
  version: number;
  issued_at: string;
  expires_at?: string;
  license?: { type: "subscription" | "lifetime" };
  updates_until?: string;
  suspended?: boolean;
  template: string;
  /** Absent from a file issued before editions. */
  edition?: Edition;
  apps: string[];
  features: string[];
  labels: Record<string, { singular: string; plural: string }>;
  declined?: { name: string; note: string }[];
}

export interface IssuedEntitlement {
  version: number;
  payload: EntitlementPayload;
  issued_by: string | null;
  issued_at: string;
}

export interface AccessRequest {
  id: string;
  installation_id: string;
  installation_name: string;
  kind: "app" | "feature";
  name: string;
  note: string;
  requested_at: string;
  status: "pending" | "approved" | "declined" | "withdrawn";
  decided_note: string;
  decided_at: string | null;
  created_at: string;
}

export interface InstallationDetail {
  installation: Installation;
  history: IssuedEntitlement[];
  requests: AccessRequest[];
  audit: AuditEntry[];
  activation?: Activation;
}

/** What goes in the installation's environment. */
export type InstallationCredential = Record<"INSTALLATION_ID" | "INSTALLATION_SECRET" | "ENTITLEMENT_PUBLIC_KEY", string>;
