"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ComponentType, SVGProps } from "react";
import type { Role } from "@/lib/types";
import { cn } from "@/lib/utils";
import { ChartIcon, ClockIcon, DashboardIcon, LockIcon, ServerIcon, SettingsIcon, TasksIcon, TemplateIcon, UsersIcon, WalletIcon } from "./icons";

type Item = { href: string; label: string; icon: ComponentType<SVGProps<SVGSVGElement>>; badge?: keyof Counts; admin?: boolean; alert?: boolean };
export interface Counts {
  leads: number;
  tasks: number;
  demos: number;
}

const ITEMS: Item[] = [
  { href: "/", label: "Overview", icon: DashboardIcon },
  { href: "/leads", label: "Leads", icon: UsersIcon, badge: "leads" },
  { href: "/tasks", label: "Tasks", icon: TasksIcon, badge: "tasks", alert: true },
  { href: "/sales", label: "Sales", icon: WalletIcon },
  { href: "/analytics", label: "Analytics", icon: ChartIcon },
  { href: "/installations", label: "Customers", icon: LockIcon },
  { href: "/releases", label: "Releases", icon: ServerIcon },
  { href: "/activity", label: "Activity", icon: ClockIcon },
  { href: "/demos", label: "Demos", icon: ServerIcon, badge: "demos" },
  { href: "/team", label: "Team", icon: UsersIcon, admin: true },
  { href: "/templates", label: "Templates", icon: TemplateIcon, admin: true },
  { href: "/settings", label: "Settings", icon: SettingsIcon, admin: true },
];

export function Nav({ counts, role }: { counts: Counts; role: Role }) {
  const path = usePathname();
  return (
    <nav aria-label="Main" className="flex gap-1 overflow-x-auto md:flex-col md:overflow-visible">
      {ITEMS.filter((i) => !i.admin || role === "admin").map(({ href, label, icon: Icon, badge, alert }) => {
        const active = href === "/" ? path === "/" : path.startsWith(href);
        const n = badge ? counts[badge] : 0;
        return (
          <Link
            key={href}
            href={href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "flex shrink-0 items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-medium transition-colors",
              active ? "bg-sidebar-accent text-foreground" : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
            )}
          >
            <Icon className="size-4" />
            {label}
            {n > 0 && <span className={cn("ml-auto rounded-full px-1.5 text-xs font-semibold", alert ? "bg-destructive/15 text-destructive" : "bg-brand/15 text-brand")}>{n}</span>}
          </Link>
        );
      })}
    </nav>
  );
}
