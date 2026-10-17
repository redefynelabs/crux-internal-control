import { ArrowLeftIcon } from "@/components/icons";
import { ButtonLink, Card, PageHeader } from "@/components/ui";
import { getMe, getUsers } from "@/lib/api";
import { NewLeadForm } from "./new-lead-form";

export const metadata = { title: "Add lead" };

export default async function NewLeadPage() {
  const [users, me] = await Promise.all([getUsers(), getMe()]);
  return (
    <>
      <ButtonLink href="/leads" variant="ghost" size="sm" className="-ml-2 mb-3"><ArrowLeftIcon className="size-4" /> Leads</ButtonLink>
      <PageHeader title="Add lead" description="Someone you met or spoke to outside the website." />
      <Card className="max-w-2xl p-5"><NewLeadForm users={users.filter((u) => u.active)} meId={me.id} /></Card>
    </>
  );
}
