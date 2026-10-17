import { notFound } from "next/navigation";
import { AdminOnly } from "@/components/admin-only";
import { ArrowLeftIcon } from "@/components/icons";
import { ButtonLink, Card, PageHeader } from "@/components/ui";
import { ApiError, getCatalog, getInstallation, getMe } from "@/lib/api";
import { EntitlementForm } from "./entitlement-form";

export const metadata = { title: "Issue entitlement" };

export default async function IssuePage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ approve?: string | string[] }> }) {
  const [{ id }, sp] = await Promise.all([params, searchParams]);
  if ((await getMe()).role !== "admin") return <AdminOnly what="Entitlements" />;
  let detail;
  try {
    detail = await getInstallation(id);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) notFound();
    throw e;
  }
  const { catalog, can_issue } = await getCatalog();
  const { installation: inst, history, requests } = detail;
  const approveIds = new Set([sp.approve ?? []].flat());
  const pending = requests.filter((r) => r.status === "pending");
  return (
    <>
      <ButtonLink href={`/installations/${id}`} variant="ghost" size="sm" className="-ml-2 mb-3"><ArrowLeftIcon className="size-4" /> {inst.name}</ButtonLink>
      <PageHeader title={`Issue entitlement`} description={`The next file for ${inst.name} will be version ${(history[0]?.version ?? 0) + 1}. Installations take it when it is uploaded or fetched.`} />
      {!can_issue ? (
        <Card className="max-w-2xl p-5 text-sm">This control plane has no signing key, so nothing can be issued. Run <code>controlplane keygen</code> and set <code>CP_ENTITLEMENT_KEY_FILE</code>.</Card>
      ) : (
        <EntitlementForm installationId={id} catalog={catalog} current={history[0]?.payload} requests={pending} preApprove={[...approveIds]} />
      )}
    </>
  );
}
