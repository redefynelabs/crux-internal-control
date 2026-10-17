import { AdminOnly } from "@/components/admin-only";
import { Badge, Card, PageHeader } from "@/components/ui";
import { getMe, getUsers } from "@/lib/api";
import { dateTime, initials, relative } from "@/lib/format";
import { InviteForm } from "./invite-form";
import { MemberActions } from "./member-actions";

export const metadata = { title: "Team" };

export default async function TeamPage() {
  const me = await getMe();
  if (me.role !== "admin") return <AdminOnly what="The team" />;
  const users = await getUsers();
  return (
    <>
      <PageHeader title="Team" description="Who can sign in. Admins manage everything; sales people work leads, follow-ups and email." />
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="overflow-hidden lg:col-span-2">
          <ul className="divide-y">
            {users.map((u) => (
              <li key={u.id} className="flex flex-wrap items-center gap-3 px-5 py-4">
                <span className="grid size-9 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">{initials(u.name)}</span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{u.name}{u.id === me.id && <span className="ml-1.5 text-xs font-normal text-muted-foreground">(you)</span>}</p>
                  <p className="truncate text-xs text-muted-foreground">{u.email}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground" title={u.last_login_at ? dateTime(u.last_login_at) : undefined}>
                    {u.pending_invite ? "Invited, not joined yet" : u.last_login_at ? `Last sign-in ${relative(u.last_login_at)}` : "Never signed in"}
                  </p>
                </div>
                {!u.active && <Badge className="bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200">Inactive</Badge>}
                {u.pending_invite && <Badge className="bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-200">Invited</Badge>}
                <MemberActions user={u} isSelf={u.id === me.id} />
              </li>
            ))}
          </ul>
        </Card>
        <Card className="h-fit p-5">
          <h2 className="font-medium">Invite someone</h2>
          <p className="mt-1 mb-4 text-sm text-muted-foreground">They get a link to set a password. It is also shown here so you can send it yourself.</p>
          <InviteForm />
        </Card>
      </div>
    </>
  );
}
