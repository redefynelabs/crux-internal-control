"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import { completeTask, rescheduleTask } from "@/app/actions-leads";
import { CheckIcon, MailIcon, MoveIcon, PhoneIcon, TasksIcon } from "@/components/icons";
import { dateTime, initials, relative } from "@/lib/format";
import type { Task } from "@/lib/types";
import { cn } from "@/lib/utils";

type Column = "overdue" | "today" | "tomorrow" | "later" | "done";

const COLUMNS: { id: Column; title: string; tone: string }[] = [
  { id: "overdue", title: "Overdue", tone: "bg-rose-500" },
  { id: "today", title: "Today", tone: "bg-amber-500" },
  { id: "tomorrow", title: "Tomorrow", tone: "bg-blue-500" },
  { id: "later", title: "Later", tone: "bg-slate-400" },
  { id: "done", title: "Done", tone: "bg-emerald-500" },
];

const KIND_ICON = { call: PhoneIcon, email: MailIcon, other: TasksIcon } as const;

function startOfDay(offset: number): Date {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  d.setDate(d.getDate() + offset);
  return d;
}

function columnOf(t: Task): Column {
  if (t.done_at) return "done";
  const due = new Date(t.due_at).getTime();
  if (due < Date.now()) return "overdue";
  if (due < startOfDay(1).getTime()) return "today";
  if (due < startOfDay(2).getTime()) return "tomorrow";
  return "later";
}

/** The time a task gets when it is dropped on a day: keep its time of day, or pick 10:00 if that has passed or is unset. */
function dueFor(col: Column, t: Task): Date {
  const old = new Date(t.due_at);
  const at = (day: Date, h = old.getHours(), m = old.getMinutes()) => {
    day.setHours(h, m, 0, 0);
    return day;
  };
  if (col === "today") {
    const d = at(startOfDay(0));
    if (d.getTime() > Date.now()) return d;
    const soon = new Date(Date.now() + 60 * 60_000);
    return soon.getDate() === new Date().getDate() ? soon : at(startOfDay(0), 23, 30);
  }
  if (col === "tomorrow") return at(startOfDay(1));
  return at(startOfDay(7), 10, 0);
}

/** Tasks as columns by when they are due. Drag a card to another day to reschedule it, to Done to complete it. */
export function TaskBoard({ tasks }: { tasks: Task[] }) {
  const router = useRouter();
  const [moved, setMoved] = useState<Record<string, Column>>({});
  const [over, setOver] = useState<Column | null>(null);
  const [dragging, setDragging] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [, start] = useTransition();

  const colOf = (t: Task) => moved[t.id] ?? columnOf(t);

  function move(t: Task, to: Column) {
    setOver(null);
    setDragging(null);
    const from = colOf(t);
    if (from === to) return;
    if (to === "overdue") {
      setError("A task cannot be moved to overdue. Pick a day instead.");
      return;
    }
    setError("");
    setMoved((m) => ({ ...m, [t.id]: to }));
    start(async () => {
      const res = to === "done" ? await completeTask(t.id, true) : await rescheduleTask(t.id, dueFor(to, t).toISOString());
      if (res.error) {
        setMoved((m) => ({ ...m, [t.id]: from }));
        setError(res.error);
      } else {
        router.refresh();
      }
    });
  }

  return (
    <>
      {error && <p role="alert" className="mb-3 rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
      <div className="grid auto-cols-[17rem] grid-flow-col gap-3 overflow-x-auto pb-3 lg:grid-flow-row lg:auto-cols-auto lg:grid-cols-5">
        {COLUMNS.map((c) => {
          const col = tasks.filter((t) => colOf(t) === c.id);
          return (
            <section
              key={c.id}
              aria-label={c.title}
              onDragOver={(e) => {
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
                if (over !== c.id) setOver(c.id);
              }}
              onDragLeave={(e) => {
                if (!e.currentTarget.contains(e.relatedTarget as Node)) setOver(null);
              }}
              onDrop={(e) => {
                e.preventDefault();
                const t = tasks.find((x) => x.id === e.dataTransfer.getData("text/plain"));
                if (t) move(t, c.id);
              }}
              className={cn("flex min-h-56 flex-col rounded-2xl bg-muted/50 p-2.5 transition-colors", over === c.id && dragging && (c.id === "overdue" ? "bg-destructive/10" : "bg-brand/10 ring-2 ring-brand/40"))}
            >
              <h2 className="flex items-center gap-2 px-1.5 pb-2 text-xs font-semibold text-muted-foreground">
                <span className={cn("size-2 rounded-full", c.tone)} />
                {c.title}
                <span className="ml-auto tabular-nums">{col.length}</span>
              </h2>
              <ul className="flex-1 space-y-2">
                {col.map((t) => {
                  const Icon = KIND_ICON[t.kind] ?? TasksIcon;
                  const done = c.id === "done";
                  return (
                    <li
                      key={t.id}
                      draggable
                      onDragStart={(e) => {
                        e.dataTransfer.setData("text/plain", t.id);
                        e.dataTransfer.effectAllowed = "move";
                        setDragging(t.id);
                      }}
                      onDragEnd={() => {
                        setDragging(null);
                        setOver(null);
                      }}
                      className={cn("group relative cursor-grab rounded-xl bg-card p-3 ring-1 ring-foreground/10 transition hover:ring-foreground/25 active:cursor-grabbing", dragging === t.id && "opacity-40")}
                    >
                      <div className="flex items-start gap-2.5">
                        <button
                          type="button"
                          aria-label={done ? `Reopen: ${t.title}` : `Mark done: ${t.title}`}
                          onClick={() => (done ? move({ ...t, done_at: null }, "today") : move(t, "done"))}
                          className={cn("mt-0.5 grid size-5 shrink-0 place-items-center rounded-md border transition-colors", done ? "border-emerald-500 bg-emerald-500 text-white" : "border-input hover:border-foreground/50")}
                        >
                          {done && <CheckIcon className="size-3.5" />}
                        </button>
                        <div className="min-w-0 flex-1 pr-5">
                          <p className={cn("text-sm font-medium", done && "text-muted-foreground line-through")}>{t.title}</p>
                          <Link href={`/leads/${t.lead_id}`} draggable={false} className="block truncate text-xs text-muted-foreground hover:text-foreground hover:underline">{t.lead_name}</Link>
                        </div>
                      </div>
                      <p className="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                        <span className={cn("inline-flex items-center gap-1", c.id === "overdue" && "font-medium text-destructive")} title={dateTime(t.due_at)}>
                          <Icon className="size-3.5" />{done && t.done_at ? `Done ${relative(t.done_at)}` : relative(t.due_at)}
                        </span>
                        {t.assignee_name && (
                          <span className="grid size-5 shrink-0 place-items-center rounded-full bg-muted text-[9px] font-semibold" title={t.assignee_name}>{initials(t.assignee_name)}</span>
                        )}
                      </p>
                      {!done && (
                    <span className="absolute top-2 right-2 grid size-6 place-items-center rounded-full bg-muted text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100">
                      <MoveIcon className="size-3.5" />
                      <select
                          aria-label={`Move ${t.title} to another day`}
                          value={c.id}
                          onChange={(e) => move(t, e.target.value as Column)}
                          className="absolute inset-0 size-full cursor-pointer opacity-0"
                        >
                          {COLUMNS.filter((x) => x.id !== "overdue" || c.id === "overdue").map((x) => <option key={x.id} value={x.id}>{x.title}</option>)}
                        </select>
                    </span>
                      )}
                    </li>
                  );
                })}
              </ul>
            </section>
          );
        })}
      </div>
    </>
  );
}
