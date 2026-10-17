import Link from "next/link";
import { notFound } from "next/navigation";
import { reactivateSale, deleteSale } from "@/app/actions-sales";
import { ArrowLeftIcon } from "@/components/icons";
import { ConfirmButton } from "@/components/confirm-button";
import { SubmitButton } from "@/components/pending-button";
import { Badge, ButtonLink, Card, PageHeader } from "@/components/ui";
import { ApiError, getMe, getSale, getUsers } from "@/lib/api";
import { BILLING_LABEL, dateOnly, rupees, SALE_PLAN_LABEL } from "@/lib/format";
import { CancelForm } from "./cancel-form";
import { SaleForm } from "../sale-form";

export const metadata = { title: "Sale" };

export default async function SalePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let sale;
  try {
    sale = await getSale(id);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) notFound();
    throw e;
  }
  const [users, me] = await Promise.all([getUsers(), getMe()]);
  const cancelled = sale.status === "cancelled";
  const recurring = sale.billing !== "one_time";
  return (
    <>
      <ButtonLink href="/sales" variant="ghost" size="sm" className="-ml-2 mb-3"><ArrowLeftIcon className="size-4" /> Sales</ButtonLink>
      <PageHeader
        title={sale.customer}
        description={[sale.company, `${BILLING_LABEL[sale.billing]}${sale.plan ? ` · ${SALE_PLAN_LABEL[sale.plan]}` : ""}`, `since ${dateOnly(sale.started_on)}`].filter(Boolean).join(" · ")}
        actions={<Badge className={cancelled ? "bg-rose-100 text-rose-800 dark:bg-rose-950 dark:text-rose-200" : "bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-200"}>{cancelled ? "Cancelled" : "Active"}</Badge>}
      />
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="p-5 lg:col-span-2"><SaleForm key={sale.created_at + sale.amount + sale.billing} sale={sale} users={users} meId={me.id} /></Card>
        <div className="space-y-4">
          <Card className="p-5">
            <h2 className="font-medium">Value</h2>
            <dl className="mt-3 space-y-3 text-sm">
              <div className="flex justify-between"><dt className="text-muted-foreground">{recurring ? `Billed ${sale.billing === "monthly" ? "each month" : "each year"}` : "Paid once"}</dt><dd className="tabular-nums">{rupees(sale.amount)}</dd></div>
              {recurring && <div className="flex justify-between"><dt className="text-muted-foreground">Monthly (MRR)</dt><dd className="tabular-nums">{rupees(sale.mrr)}</dd></div>}
              {recurring && <div className="flex justify-between"><dt className="text-muted-foreground">Yearly (ARR)</dt><dd className="tabular-nums">{rupees(sale.mrr * 12)}</dd></div>}
              {sale.lead_id && <div className="flex justify-between"><dt className="text-muted-foreground">Lead</dt><dd><Link className="hover:underline" href={`/leads/${sale.lead_id}`}>Open</Link></dd></div>}
            </dl>
          </Card>
          <Card className="p-5">
            <h2 className="font-medium">{cancelled ? "Cancelled" : "Cancel this sale"}</h2>
            {cancelled ? (
              <>
                <p className="mt-2 text-sm text-muted-foreground">Ended {sale.ended_on ? dateOnly(sale.ended_on) : ""}{sale.cancel_reason ? `. ${sale.cancel_reason}` : "."}</p>
                <form action={reactivateSale.bind(null, sale.id)} className="mt-3"><SubmitButton variant="secondary" size="sm" pending="…">Reactivate</SubmitButton></form>
              </>
            ) : (
              <CancelForm id={sale.id} recurring={recurring} />
            )}
          </Card>
          {me.role === "admin" && (
            <Card className="p-5">
              <h2 className="font-medium">Delete</h2>
              <p className="mt-2 text-sm text-muted-foreground">Only for a sale recorded by mistake. It leaves the numbers. To keep the history, cancel it instead.</p>
              <form action={deleteSale.bind(null, sale.id)} className="mt-3"><ConfirmButton message="Delete this sale for good?">Delete</ConfirmButton></form>
            </Card>
          )}
        </div>
      </div>
    </>
  );
}
