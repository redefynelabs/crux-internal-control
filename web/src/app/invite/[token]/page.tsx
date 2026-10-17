import type { Metadata } from "next";
import { ApiError, publicRequest } from "@/lib/api";
import { Card } from "@/components/ui";
import { InviteForm } from "./invite-form";

export const metadata: Metadata = { title: "Join the team" };
export const dynamic = "force-dynamic";

export default async function InvitePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  let invite: { email: string; name: string } | null = null;
  try {
    invite = await publicRequest<{ email: string; name: string }>(`/v1/auth/invite/${encodeURIComponent(token)}`);
  } catch (e) {
    if (!(e instanceof ApiError && (e.status === 404 || e.status === 410))) throw e;
  }
  return (
    <div className="grid min-h-screen place-items-center px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="grid size-11 place-items-center rounded-2xl bg-primary text-lg font-bold text-primary-foreground">C</span>
          <h1 className="mt-4 text-2xl font-semibold tracking-tight">Join the control plane</h1>
          <p className="mt-1 text-sm text-muted-foreground">{invite ? `Choose a password for ${invite.email}.` : "This invite link is not valid."}</p>
        </div>
        {invite ? (
          <InviteForm token={token} name={invite.name} />
        ) : (
          <Card className="p-6 text-sm text-muted-foreground">The link may have expired or already been used. Ask an admin to send you a new invite.</Card>
        )}
      </div>
    </div>
  );
}
