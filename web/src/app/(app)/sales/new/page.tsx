import { redirect } from "next/navigation";
import { ArrowLeftIcon } from "@/components/icons";
import { ButtonLink, Card, PageHeader } from "@/components/ui";
import { ApiError, getLead, getMe, getUsers } from "@/lib/api";
import type { Sale } from "@/lib/types";
import { SaleForm } from "../sale-form";

export const metadata = { title: "Record sale" };

export default async function NewSalePage({ searchParams }: { searchParams: Promise<{ lead?: string }> }) {
  const { lead: leadId } = await searchParams;
  const [users, me] = await Promise.all([getUsers(), getMe()]);
  let prefill: Partial<Sale> | undefined;
  let existing: string | undefined;
  if (leadId) {
    try {
      const { lead, sale } = await getLead(leadId);
      existing = sale?.id;
      prefill = existing ? undefined : {
        lead_id: lead.id, customer: lead.name, company: lead.company, email: lead.email, plan: lead.plan_interest,
        billing: lead.interest === "cloud" ? "monthly" : "one_time", owner_id: lead.owner_id ?? undefined,
      };
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 404)) throw e;
    }
  }
  if (existing) redirect(`/sales/${existing}`);
  return (
    <>
      <ButtonLink href="/sales" variant="ghost" size="sm" className="-ml-2 mb-3"><ArrowLeftIcon className="size-4" /> Sales</ButtonLink>
      <PageHeader title="Record sale" description={prefill ? "Closing this lead as won. Check the details and the amount." : "A customer who bought, or signed up for a plan."} />
      <Card className="max-w-2xl p-5"><SaleForm prefill={prefill} users={users} meId={me.id} /></Card>
    </>
  );
}
