import Link from "next/link";
import { setTaskDone } from "@/app/actions-leads";
import type { Task } from "@/lib/types";
import { dateTime, isPast, relative } from "@/lib/format";
import { cn } from "@/lib/utils";
import { CheckIcon, MailIcon, PhoneIcon, TasksIcon } from "./icons";

const KIND_ICON = { call: PhoneIcon, email: MailIcon, other: TasksIcon } as const;

/** One task with a button to tick it off. showLead links to the lead (the tasks page); on a lead it is omitted. */
export function TaskItem({ task, showLead = false, leadId }: { task: Task; showLead?: boolean; leadId?: string }) {
  const done = !!task.done_at;
  const over = !done && isPast(task.due_at);
  const Icon = KIND_ICON[task.kind] ?? TasksIcon;
  return (
    <li className="flex items-start gap-3 py-3 first:pt-0 last:pb-0">
      <form action={setTaskDone.bind(null, task.id, leadId ?? task.lead_id, !done)}>
        <button
          type="submit"
          aria-label={done ? `Reopen: ${task.title}` : `Mark done: ${task.title}`}
          className={cn("mt-0.5 grid size-5 place-items-center rounded-md border transition-colors", done ? "border-emerald-500 bg-emerald-500 text-white" : "border-input hover:border-foreground/50")}
        >
          {done && <CheckIcon className="size-3.5" />}
        </button>
      </form>
      <div className="min-w-0 flex-1">
        <p className={cn("text-sm font-medium", done && "text-muted-foreground line-through")}>{task.title}</p>
        <p className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
          <Icon className="size-3.5" />
          <span className={cn(over && "font-medium text-destructive")} title={dateTime(task.due_at)}>{done ? `Done ${relative(task.done_at!)}` : `${over ? "Overdue · " : "Due "}${relative(task.due_at)}`}</span>
          {task.assignee_name && <span>· {task.assignee_name}</span>}
          {showLead && <Link href={`/leads/${task.lead_id}`} className="hover:text-foreground hover:underline">· {task.lead_name}</Link>}
        </p>
      </div>
    </li>
  );
}
