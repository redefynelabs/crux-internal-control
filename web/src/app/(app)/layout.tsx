import type { ReactNode } from "react";
import { redirect } from "next/navigation";
import { logout } from "@/app/actions";
import { Nav } from "@/components/nav";
import { LogoutIcon } from "@/components/icons";
import { getMe, getStats, sessionToken } from "@/lib/api";
import { initials, ROLE_LABEL } from "@/lib/format";

export const dynamic = "force-dynamic";

export default async function AppLayout({ children }: { children: ReactNode }) {
  if (!(await sessionToken())) redirect("/login");
  const [me, stats] = await Promise.all([getMe(), getStats()]);
  const counts = { leads: stats.leads.new ?? 0, tasks: stats.my_overdue_tasks, demos: stats.demos.active ?? 0 };
  return (
    <div className="min-h-screen md:grid md:grid-cols-[15rem_1fr]">
      <aside className="border-b border-sidebar-border bg-sidebar text-sidebar-foreground md:sticky md:top-0 md:flex md:h-screen md:flex-col md:border-r md:border-b-0">
        <div className="flex items-center justify-between gap-3 px-4 py-3 md:px-5 md:py-5">
          <div className="flex items-center gap-2.5">
            <span className="grid size-8 place-items-center rounded-xl bg-primary text-sm font-bold text-primary-foreground">C</span>
            <div className="leading-tight">
              <p className="text-sm font-semibold">Control plane</p>
              <p className="text-xs text-muted-foreground">Back office</p>
            </div>
          </div>
          <form action={logout} className="md:hidden">
            <button type="submit" aria-label="Sign out" className="rounded-lg p-2 text-muted-foreground hover:bg-sidebar-accent"><LogoutIcon className="size-4" /></button>
          </form>
        </div>
        <div className="px-3 pb-3 md:flex-1 md:px-3">
          <Nav counts={counts} role={me.role} />
        </div>
        <div className="hidden border-t border-sidebar-border p-3 md:block">
          <div className="flex items-center gap-2.5 px-3 pb-2">
            <span className="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">{initials(me.name)}</span>
            <div className="min-w-0 leading-tight">
              <p className="truncate text-sm font-medium">{me.name}</p>
              <p className="text-xs text-muted-foreground">{ROLE_LABEL[me.role]}</p>
            </div>
          </div>
          <form action={logout}>
            <button type="submit" className="flex w-full items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-sidebar-accent/60 hover:text-foreground">
              <LogoutIcon className="size-4" /> Sign out
            </button>
          </form>
        </div>
      </aside>
      <main className="min-w-0 px-4 py-6 sm:px-8 sm:py-8">
        <div className="mx-auto max-w-6xl">{children}</div>
      </main>
    </div>
  );
}
