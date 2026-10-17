import { AdminOnly } from "@/components/admin-only";
import { ArrowLeftIcon } from "@/components/icons";
import { ButtonLink, Card, PageHeader } from "@/components/ui";
import { getMe } from "@/lib/api";
import { InstallationForm } from "./installation-form";

export const metadata = { title: "Add customer" };

export default async function NewInstallationPage() {
  if ((await getMe()).role !== "admin") return <AdminOnly what="Entitlements" />;
  return (
    <>
      <ButtonLink href="/installations" variant="ghost" size="sm" className="-ml-2 mb-3"><ArrowLeftIcon className="size-4" /> Entitlements</ButtonLink>
      <PageHeader title="Add customer" description="One entry for each ERP installation. You issue its entitlement next." />
      <Card className="max-w-xl p-5"><InstallationForm /></Card>
    </>
  );
}
