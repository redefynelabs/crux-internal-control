import { AdminOnly } from "@/components/admin-only";
import { Card, PageHeader } from "@/components/ui";
import { getMe, getTemplates } from "@/lib/api";
import { relative } from "@/lib/format";
import { TemplateForm } from "./template-form";

export const metadata = { title: "Templates" };

const PLACEHOLDERS: [string, string][] = [
  ["{{name}}", "Their full name"],
  ["{{first_name}}", "Their first name"],
  ["{{company}}", "Their company"],
  ["{{demo_url}}", "Link to their running demo"],
  ["{{owner}}", "The lead's owner"],
  ["{{signature}}", "Your signature from the email settings"],
  ["{{site_url}}", "Your website, for demo and pricing links"],
  ["{{sender_email}}", "Where replies go, for reply-by-email buttons"],
];

/** Only the emails the control plane sends by itself have these. */
const SYSTEM_PLACEHOLDERS: [string, string][] = [
  ["{{login_email}}", "The demo sign-in email"],
  ["{{login_password}}", "The demo sign-in password"],
  ["{{ends_at}}", "When the demo ends (UTC)"],
  ["{{minutes}}", "How long the demo runs"],
];

export default async function TemplatesPage() {
  const me = await getMe();
  if (me.role !== "admin") return <AdminOnly what="Email templates are" />;
  const templates = await getTemplates();
  return (
    <>
      <PageHeader title="Email templates" description="The emails sales sends from a lead, and the ones the control plane sends by itself. A designed email gets the Crux logo and layout automatically." />
      <Card className="mb-4 p-5">
        <h2 className="text-sm font-medium">Placeholders</h2>
        <dl className="mt-3 grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
          {PLACEHOLDERS.map(([k, v]) => (
            <div key={k} className="flex items-baseline gap-2"><dt className="font-mono text-xs">{k}</dt><dd className="text-muted-foreground">{v}</dd></div>
          ))}
        </dl>
        <p className="mt-4 text-xs text-muted-foreground">Only in “Demo ready (after waiting)”:</p>
        <dl className="mt-2 grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
          {SYSTEM_PLACEHOLDERS.map(([k, v]) => (
            <div key={k} className="flex items-baseline gap-2"><dt className="font-mono text-xs">{k}</dt><dd className="text-muted-foreground">{v}</dd></div>
          ))}
        </dl>
      </Card>
      <div className="space-y-4">
        {templates.map((t) => (
          <Card key={t.id} className="p-5">
            <div className="mb-4 flex items-baseline justify-between gap-3">
              <h2 className="font-medium">{t.name}</h2>
              <p className="text-xs text-muted-foreground">Edited {relative(t.updated_at)}</p>
            </div>
            <TemplateForm template={t} />
          </Card>
        ))}
        {templates.length === 0 && <Card className="p-5 text-sm text-muted-foreground">No templates yet.</Card>}
      </div>
    </>
  );
}
