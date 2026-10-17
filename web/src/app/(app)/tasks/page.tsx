import Link from "next/link";
import { TaskItem } from "@/components/task-item";
import { Button, Card, EmptyState, PageHeader, Select } from "@/components/ui";
import { BoardIcon, ListIcon } from "@/components/icons";
import { getLeads, getMe, getTasks, getUsers } from "@/lib/api";
import type { Task } from "@/lib/types";
import { cn } from "@/lib/utils";
import { NewTask } from "./new-task";
import { TaskBoard } from "./task-board";

export const metadata = { title: "Tasks" };

/** Splits open tasks by when they are due, relative to now. */
function bucket(tasks: Task[]) {
  const now = Date.now();
  const startOfTomorrow = new Date();
  startOfTomorrow.setHours(24, 0, 0, 0);
  const due = (t: Task) => new Date(t.due_at).getTime();
  return {
    overdue: tasks.filter((t) => due(t) < now),
    today: tasks.filter((t) => due(t) >= now && due(t) < startOfTomorrow.getTime()),
    upcoming: tasks.filter((t) => due(t) >= startOfTomorrow.getTime()),
  };
}

export default async function TasksPage({ searchParams }: { searchParams: Promise<{ assignee?: string; state?: string; view?: string }> }) {
  const p = await searchParams;
  const assignee = p.assignee || "me";
  const board = p.view === "board";
  // The board shows open and recently completed tasks together; the list shows one state at a time.
  const done = !board && p.state === "done";
  const [users, me, leads, open, finished] = await Promise.all([
    getUsers(),
    getMe(),
    getLeads({ limit: 500 }),
    done ? Promise.resolve([] as Task[]) : getTasks(assignee, "open", 300),
    done || board ? getTasks(assignee, "done", board ? 15 : 200) : Promise.resolve([] as Task[]),
  ]);
  const tasks = done ? finished : open;

  const { overdue, today, upcoming } = bucket(tasks);
  const groups: { title: string; tone?: string; items: Task[] }[] = done
    ? [{ title: "Completed", items: tasks }]
    : [
        { title: "Overdue", tone: "text-destructive", items: overdue },
        { title: "Today", items: today },
        { title: "Upcoming", items: upcoming },
      ];
  const href = (over: { assignee?: string; state?: string; view?: string }) => {
    const next = { assignee, state: done ? "done" : undefined, view: board ? "board" : undefined, ...over };
    const qs = Object.entries(next).filter(([, v]) => v && v !== "open").map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`).join("&");
    return `/tasks${qs ? `?${qs}` : ""}`;
  };

  return (
    <>
      <PageHeader
        title="Tasks"
        description="Follow-ups across all leads: calls to make, emails to send, proposals to chase."
        actions={<NewTask leads={leads} users={users.filter((u) => u.active)} meId={me.id} />}
      />

      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
        {!board && <nav aria-label="Task state" className="flex gap-1.5">
          {[{ id: "open", label: "Open" }, { id: "done", label: "Completed" }].map((t) => {
            const active = (t.id === "done") === done;
            return (
              <Link key={t.id} href={href({ state: t.id })} aria-current={active ? "page" : undefined} className={cn("inline-flex h-8 items-center rounded-4xl px-3 text-sm font-medium transition-colors", active ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}>{t.label}</Link>
            );
          })}
        </nav>}
        <div className="flex items-center gap-2">
          <Link href={href({ view: undefined })} aria-label="List view" aria-current={!board ? "page" : undefined} className={cn("grid size-8 place-items-center rounded-4xl", !board ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}><ListIcon className="size-4" /></Link>
          <Link href={href({ view: "board", state: undefined })} aria-label="Board view" aria-current={board ? "page" : undefined} className={cn("grid size-8 place-items-center rounded-4xl", board ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground")}><BoardIcon className="size-4" /></Link>
        </div>
        </div>
        <form action="/tasks" className="flex items-center gap-2">
          {done && <input type="hidden" name="state" value="done" />}
          {board && <input type="hidden" name="view" value="board" />}
          <Select name="assignee" defaultValue={assignee} aria-label="Assigned to" className="w-48">
            <option value="me">My tasks</option>
            <option value="all">Everyone</option>
            {users.filter((u) => u.active).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
          </Select>
          <Button type="submit" variant="secondary">Apply</Button>
        </form>
      </div>

      {board ? (
        <TaskBoard tasks={[...open, ...finished]} />
      ) : tasks.length === 0 ? (
        <Card><EmptyState title={done ? "Nothing completed yet" : "You are all caught up"} text={done ? undefined : "Add a task from any lead to see it here."} /></Card>
      ) : (
        <div className="space-y-4">
          {groups.filter((g) => g.items.length > 0).map((g) => (
            <Card key={g.title} className="p-5">
              <h2 className={cn("font-medium", g.tone)}>{g.title} <span className="text-sm font-normal text-muted-foreground tabular-nums">{g.items.length}</span></h2>
              <ul className="mt-3 divide-y">{g.items.map((t) => <TaskItem key={t.id} task={t} showLead />)}</ul>
            </Card>
          ))}
        </div>
      )}
    </>
  );
}
