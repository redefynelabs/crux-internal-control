import type { ComponentType, SVGProps } from "react";
import { AlertIcon, CheckIcon, ClockIcon, MailIcon, NoteIcon, PhoneIcon, ServerIcon, TasksIcon, UsersIcon } from "@/components/icons";
import { dateTime, INTEREST_LABEL, PLAN_LABEL, relative, rupees, STATUS_LABEL } from "@/lib/format";
import type { LeadEvent, SentEmail } from "@/lib/types";

type Icon = ComponentType<SVGProps<SVGSVGElement>>;
interface Item {
  key: string;
  at: string;
  icon: Icon;
  tone?: "bad";
  title: string;
  text?: string | null;
  who?: string;
  email?: SentEmail;
}

const CALL_OUTCOME: Record<string, string> = { reached: "Spoke to them", no_answer: "No answer", voicemail: "Left a voicemail", wrong_number: "Wrong number" };
const s = (v: unknown) => (typeof v === "string" ? v : "");

function describe(e: LeadEvent, i: number): Item {
  const d = e.detail ?? {};
  const base = { key: `e${i}`, at: e.at, who: e.actor_name };
  switch (e.kind) {
    case "lead_created": return { ...base, icon: UsersIcon, title: "Lead added" };
    case "demo_started": return { ...base, icon: ServerIcon, title: "Started a demo", text: [s(d.slug), d.from_queue ? `after waiting ${s(d.waited)}` : ""].filter(Boolean).join(" · ") };
    case "demo_queued": return { ...base, icon: ClockIcon, title: "Waiting for a free demo room", text: typeof d.position === "number" ? `Place ${d.position} in line` : null };
    case "demo_queue_expired": return { ...base, icon: AlertIcon, tone: "bad", title: "Gave up waiting for a demo room", text: "They were sent an apology." };
    case "demo_cancelled": return { ...base, icon: ServerIcon, title: "Demo request cancelled" };
    case "demo_failed": return { ...base, icon: AlertIcon, tone: "bad", title: "The demo could not be started", text: "They were sent an apology." };
    case "demo_ended": return { ...base, icon: ServerIcon, title: "Demo ended" };
    case "status_changed": return { ...base, icon: ClockIcon, title: `Status: ${STATUS_LABEL[s(d.status) as keyof typeof STATUS_LABEL] ?? s(d.status).replace(/_/g, " ")}`, text: s(d.from) ? `from ${STATUS_LABEL[s(d.from) as keyof typeof STATUS_LABEL] ?? s(d.from)}` : null };
    case "contact_message": {
      const meta = [INTEREST_LABEL[s(d.interest)], PLAN_LABEL[s(d.plan)], s(d.team_size) && `${s(d.team_size)} people`].filter(Boolean).join(" · ");
      return { ...base, icon: MailIcon, title: "Sent a message", text: [s(d.message), meta].filter(Boolean).join("\n\n") };
    }
    case "note": return { ...base, icon: NoteIcon, title: "Note", text: s(d.text) };
    case "call": return { ...base, icon: PhoneIcon, title: `Call: ${CALL_OUTCOME[s(d.outcome)] ?? s(d.outcome)}`, text: s(d.text) };
    case "email_sent": return { ...base, icon: MailIcon, title: `Email sent: ${s(d.subject)}`, text: s(d.to) ? `to ${s(d.to)}` : null };
    case "email_failed": return { ...base, icon: AlertIcon, tone: "bad", title: `Email failed: ${s(d.subject)}`, text: s(d.error) };
    case "owner_changed": return { ...base, icon: UsersIcon, title: s(d.owner_name) ? `Assigned to ${s(d.owner_name)}` : "Unassigned" };
    case "task_added": return { ...base, icon: TasksIcon, title: `Task added: ${s(d.title)}`, text: s(d.due_at) ? `due ${dateTime(s(d.due_at))}` : null };
    case "task_done": return { ...base, icon: TasksIcon, title: `Task done: ${s(d.title)}` };
    case "sale_recorded": return { ...base, icon: CheckIcon, title: "Sale recorded", text: typeof d.amount === "number" ? `${rupees(d.amount)}${d.billing === "monthly" ? " a month" : d.billing === "yearly" ? " a year" : ""}` : null };
    case "sale_cancelled": return { ...base, icon: AlertIcon, tone: "bad", title: "Sale cancelled", text: s(d.reason) || null };
    case "sale_reactivated": return { ...base, icon: CheckIcon, title: "Sale reactivated" };
    default: return { ...base, icon: ClockIcon, title: e.kind.replace(/_/g, " ") };
  }
}

/** Events and emails as one list, newest first. A sent email's body opens under its event. */
export function Timeline({ events, emails }: { events: LeadEvent[]; emails: SentEmail[] }) {
  const used = new Set<string>();
  const items: Item[] = events.map((e, i) => {
    const item = describe(e, i);
    if (e.kind === "email_sent") {
      const match = emails.find((m) => !used.has(m.id) && m.subject === s(e.detail?.subject) && Math.abs(new Date(m.created_at).getTime() - new Date(e.at).getTime()) < 5 * 60_000);
      if (match) {
        used.add(match.id);
        item.email = match;
      }
    }
    return item;
  });
  // Queued mail has no event yet; show it so a sent email never seems to vanish.
  for (const m of emails) {
    if (m.status === "queued" && !used.has(m.id)) items.push({ key: `m${m.id}`, at: m.created_at, icon: MailIcon, title: `Email queued: ${m.subject}`, text: `to ${m.to_email}`, who: m.sent_by_name, email: m });
  }
  items.sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime());
  if (items.length === 0) return <p className="mt-3 text-sm text-muted-foreground">Nothing has happened yet.</p>;
  return (
    <ol className="mt-4 space-y-5 border-l pl-6">
      {items.map((it) => {
        const Icon = it.icon;
        return (
          <li key={it.key} className="relative">
            <span className={`absolute top-0 -left-[2.2rem] grid size-6 place-items-center rounded-full ring-4 ring-background ${it.tone === "bad" ? "bg-destructive/15 text-destructive" : "bg-muted text-muted-foreground"}`}><Icon className="size-3.5" /></span>
            <p className="text-sm font-medium">{it.title}</p>
            {it.text && <p className="mt-0.5 text-sm break-words whitespace-pre-wrap text-muted-foreground">{it.text}</p>}
            {it.email && (
              <details className="mt-1 text-sm">
                <summary className="cursor-pointer text-xs text-muted-foreground hover:text-foreground">Show message</summary>
                <p className="mt-1 rounded-xl bg-muted/60 p-3 break-words whitespace-pre-wrap">{it.email.body}</p>
              </details>
            )}
            <p className="mt-0.5 text-xs text-muted-foreground" title={dateTime(it.at)}>{[it.who, relative(it.at)].filter(Boolean).join(" · ")}</p>
          </li>
        );
      })}
    </ol>
  );
}
