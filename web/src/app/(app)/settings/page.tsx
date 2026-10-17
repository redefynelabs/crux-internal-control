import Link from "next/link";
import { AdminOnly } from "@/components/admin-only";
import { Card, PageHeader } from "@/components/ui";
import { getEmailSettings, getGeneralSettings, getMe } from "@/lib/api";
import { cn } from "@/lib/utils";
import { AccountForm } from "./account-form";
import { EmailForm, TestEmailForm } from "./email-form";
import { GeneralForm } from "./general-form";

export const metadata = { title: "Settings" };

const TABS = [
  { id: "email", label: "Email" },
  { id: "general", label: "General" },
  { id: "account", label: "Account" },
] as const;

export default async function SettingsPage({ searchParams }: { searchParams: Promise<{ tab?: string }> }) {
  const me = await getMe();
  if (me.role !== "admin") return <AdminOnly what="Settings are" />;
  const { tab: raw } = await searchParams;
  const tab = TABS.find((t) => t.id === raw)?.id ?? "email";
  const [email, general] = await Promise.all([tab === "email" ? getEmailSettings() : null, tab === "general" ? getGeneralSettings() : null]);

  return (
    <>
      <PageHeader title="Settings" description="How the control plane sends email and handles new leads." />
      <nav aria-label="Settings" className="mb-4 flex gap-1 border-b">
        {TABS.map((t) => (
          <Link
            key={t.id}
            href={`/settings?tab=${t.id}`}
            aria-current={tab === t.id ? "page" : undefined}
            className={cn("-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors", tab === t.id ? "border-primary text-foreground" : "border-transparent text-muted-foreground hover:text-foreground")}
          >
            {t.label}
          </Link>
        ))}
      </nav>

      {tab === "email" && email && (
        <div className="grid gap-4 lg:grid-cols-3">
          <Card className="p-5 lg:col-span-2">
            <h2 className="font-medium">SMTP</h2>
            <p className="mt-1 mb-4 text-sm text-muted-foreground">Emails to leads are sent through this mail server. The password is stored encrypted and never shown again.</p>
            <EmailForm settings={email} />
          </Card>
          <div className="space-y-4">
            <Card className="p-5">
              <h2 className="font-medium">Send a test</h2>
              <p className="mt-1 mb-4 text-sm text-muted-foreground">Saves nothing. Uses the settings already saved.</p>
              <TestEmailForm defaultTo={me.email} />
            </Card>
            <Card className="p-5">
              <h2 className="font-medium">Common providers</h2>
              <dl className="mt-3 space-y-2.5 text-sm">
                {[
                  ["Google Workspace", "smtp.gmail.com · 587 · STARTTLS (use an app password)"],
                  ["Zoho Mail", "smtp.zoho.in · 587 · STARTTLS"],
                  ["Amazon SES", "email-smtp.<region>.amazonaws.com · 587 · STARTTLS"],
                  ["Resend", "smtp.resend.com · 465 · TLS (user: resend)"],
                  ["Brevo", "smtp-relay.brevo.com · 587 · STARTTLS"],
                ].map(([k, v]) => (
                  <div key={k}><dt className="font-medium">{k}</dt><dd className="text-xs text-muted-foreground">{v}</dd></div>
                ))}
              </dl>
            </Card>
          </div>
        </div>
      )}

      {tab === "general" && general && (
        <Card className="max-w-2xl p-5">
          <GeneralForm settings={general} />
        </Card>
      )}

      {tab === "account" && (
        <Card className="max-w-md p-5">
          <h2 className="font-medium">Change my password</h2>
          <p className="mt-1 mb-4 text-sm text-muted-foreground">{me.name} · {me.email}</p>
          <AccountForm />
        </Card>
      )}
    </>
  );
}
