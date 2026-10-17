"use client";

import { useActionState, useEffect, useRef, useState } from "react";
import { issueEntitlement } from "@/app/actions-entitlements";
import type { FormState } from "@/lib/form";
import { Field, SavedNote } from "@/components/field";
import { SubmitButton } from "@/components/pending-button";
import { Button, Card, Input, Select } from "@/components/ui";
import type { AccessRequest, EntitlementCatalog, EntitlementGroup, EntitlementPayload } from "@/lib/types";

/** What a request asks for, as the apps and features to switch on: an app brings its features, a feature brings its app. */
function grantedBy(catalog: EntitlementCatalog, r: AccessRequest) {
  if (r.kind === "app") {
    const app = catalog.apps.find((a) => a.name === r.name);
    return { apps: [r.name], features: app?.features.map((f) => f.name) ?? [] };
  }
  const app = catalog.apps.find((a) => a.features.some((f) => f.name === r.name));
  return { apps: app ? [app.name] : [], features: [r.name] };
}

export function EntitlementForm({ installationId, catalog, current, requests, preApprove }: {
  installationId: string;
  catalog: EntitlementCatalog;
  current?: EntitlementPayload;
  requests: AccessRequest[];
  preApprove: string[];
}) {
  const defaults = Object.fromEntries(catalog.records.map((r) => [r.key, { singular: r.singular, plural: r.plural }]));
  const [state, action] = useActionState<FormState, FormData>(issueEntitlement.bind(null, installationId, defaults), {});
  const f = state.fields ?? {};

  const [approve, setApprove] = useState<string[]>(preApprove.filter((id) => requests.some((r) => r.id === id)));
  const [apps, setApps] = useState<Set<string>>(() => {
    const s = new Set(current?.apps ?? []);
    for (const r of requests.filter((r) => preApprove.includes(r.id))) grantedBy(catalog, r).apps.forEach((a) => s.add(a));
    return s;
  });
  const [features, setFeatures] = useState<Set<string>>(() => {
    const s = new Set(current?.features ?? (current ? [] : catalog.core.map((c) => c.name)));
    for (const r of requests.filter((r) => preApprove.includes(r.id))) grantedBy(catalog, r).features.forEach((x) => s.add(x));
    return s;
  });

  const toggleApp = (app: EntitlementCatalog["apps"][number], on: boolean) => {
    setApps((p) => { const n = new Set(p); if (on) n.add(app.name); else n.delete(app.name); return n; });
    // A new app starts with all its features; removing one removes them, so no feature is left without its app.
    setFeatures((p) => { const n = new Set(p); for (const x of app.features) { if (on) n.add(x.name); else n.delete(x.name); } return n; });
  };
  const toggleFeature = (app: EntitlementCatalog["apps"][number] | undefined, name: string, on: boolean) => {
    setFeatures((p) => { const n = new Set(p); if (on) n.add(name); else n.delete(name); return n; });
    if (on && app) setApps((p) => new Set(p).add(app.name));
  };
  const toggleRequest = (r: AccessRequest, on: boolean) => {
    setApprove((p) => (on ? [...p, r.id] : p.filter((x) => x !== r.id)));
    if (!on) return;
    const g = grantedBy(catalog, r);
    setApps((p) => new Set([...p, ...g.apps]));
    setFeatures((p) => new Set([...p, ...g.features]));
  };

  const [note, setNote] = useState("");
  const [license, setLicense] = useState<"subscription" | "lifetime">(current?.license?.type ?? (current && !current.expires_at ? "lifetime" : "subscription"));
  const groupState = (g: EntitlementGroup): "on" | "partly" | "off" => {
    const have = g.features.filter((x) => features.has(x)).length;
    if (have === g.features.length && g.apps.every((a) => apps.has(a))) return "on";
    return have > 0 || g.apps.some((a) => apps.has(a) && g.features.length === 0) ? "partly" : "off";
  };
  // Ticking a group switches on its apps and features, plus the apps those need, and says so. Unticking removes the
  // group's features, and an app only once none of its features is left and no other chosen app needs it.
  const toggleGroup = (g: EntitlementGroup, on: boolean) => {
    const na = new Set(apps);
    const nf = new Set(features);
    const added: string[] = [];
    if (on) {
      for (const a of g.apps) {
        na.add(a);
        for (const need of catalog.apps.find((x) => x.name === a)?.needs ?? []) {
          if (!na.has(need)) {
            na.add(need);
            added.push(catalog.apps.find((x) => x.name === need)?.label ?? need);
          }
        }
      }
      g.features.forEach((x) => nf.add(x));
    } else {
      g.features.forEach((x) => nf.delete(x));
      for (const a of g.apps) {
        const app = catalog.apps.find((x) => x.name === a);
        const left = app?.features.some((x) => nf.has(x.name));
        const needed = [...na].some((o) => o !== a && catalog.apps.find((x) => x.name === o)?.needs?.includes(a));
        if (!left && !needed) na.delete(a);
      }
    }
    setApps(na);
    setFeatures(nf);
    setNote(added.length ? `${g.label} also needs ${added.join(" and ")}, so it was switched on too.` : "");
  };

  const [words, setWords] = useState<Record<string, { singular: string; plural: string }>>(() =>
    Object.fromEntries(catalog.records.map((r) => [r.key, current?.labels?.[r.key] ?? { singular: r.singular, plural: r.plural }])));
  const [templateId, setTemplateId] = useState(current?.template ?? catalog.templates[0]?.id ?? "");
  const template = catalog.templates.find((t) => t.id === templateId);
  // The industry's apps, features and words replace what is chosen. Features the ERP does not have yet are left out.
  const useTemplate = () => {
    if (!template) return;
    const known = new Set([...catalog.core.map((c) => c.name), ...catalog.apps.flatMap((a) => [a.name, ...a.features.map((x) => x.name)])]);
    setApps(new Set(template.apps.filter((a) => known.has(a))));
    setFeatures(new Set(template.features.filter((x) => known.has(x))));
    setWords(Object.fromEntries(catalog.records.map((r) => [r.key, template.labels[r.key] ?? { singular: r.singular, plural: r.plural }])));
  };
  return (
    <form action={action} className="space-y-4">
      {requests.length > 0 && (
        <Card className="p-5">
          <h2 className="font-medium">Requests this file answers</h2>
          <p className="mt-1 text-xs text-muted-foreground">Ticked requests are marked approved when you issue. Ticking one switches on what it asks for.</p>
          <ul className="mt-3 space-y-2 text-sm">
            {requests.map((r) => (
              <li key={r.id}>
                <label className="flex items-start gap-2.5">
                  <input type="checkbox" name="approve" value={r.id} checked={approve.includes(r.id)} onChange={(e) => toggleRequest(r, e.target.checked)} className="mt-1 size-4" />
                  <span><code className="rounded bg-muted px-1.5 py-0.5 text-xs">{r.name}</code>{r.note && <span className="text-muted-foreground"> {r.note}</span>}</span>
                </label>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <Card className="grid gap-4 p-5 sm:grid-cols-2">
        <Field id="template" label="Template" error={f.template} hint="Fields, statuses and pipelines the first administrator starts with. Applied once.">
          <Select id="template" name="template" value={templateId} onChange={(e) => setTemplateId(e.target.value)} required>
            {catalog.templates.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </Select>
          {template && <p className="mt-1.5 text-xs text-muted-foreground">{template.summary}</p>}
          <Button type="button" variant="secondary" size="sm" className="mt-2" onClick={useTemplate}>Use this template&apos;s apps, features and words</Button>
        </Field>
        <Field id="license" label="Licence" error={f.license} hint="A lifetime licence never ends. A subscription is read-only 14 days after its end date.">
          <Select id="license" name="license" value={license} onChange={(e) => setLicense(e.target.value as "subscription" | "lifetime")}>
            <option value="subscription">Subscription</option>
            <option value="lifetime">Lifetime</option>
          </Select>
        </Field>
        {license === "subscription" && (
          <Field id="expires_on" label="Subscription ends on" error={f.expires_at}>
            <Input id="expires_on" name="expires_on" type="date" defaultValue={current?.expires_at?.slice(0, 10) ?? ""} required />
          </Field>
        )}
        <Field id="updates_on" label="Updates included until (AMC)" error={f.updates_until} hint="Releases published after this date are not offered. The software keeps running.">
          <Input id="updates_on" name="updates_on" type="date" defaultValue={current?.updates_until?.slice(0, 10) ?? ""} />
        </Field>
      </Card>

      <Card className="p-5">
        <h2 className="font-medium">What the business needs</h2>
        <p className="mt-1 text-xs text-muted-foreground">Tick the outcomes this customer is paying for. The apps and features behind them are switched on below, where you can still fine-tune.</p>
        <ul className="mt-3 grid gap-2 sm:grid-cols-2">
          {catalog.groups.map((g) => (
            <GroupRow key={g.key} group={g} state={groupState(g)} onChange={(on) => toggleGroup(g, on)} />
          ))}
        </ul>
        {note && <p role="status" className="mt-3 text-sm text-muted-foreground">{note}</p>}
      </Card>

      <Card className="p-5">
        <h2 className="font-medium">Fine-tune apps and features</h2>
        {(f.apps || f.features) && <p role="alert" className="mt-2 text-sm text-destructive">{f.apps ?? f.features}</p>}
        <div className="mt-3 grid gap-4 sm:grid-cols-2">
          <fieldset className="rounded-xl border p-4">
            <legend className="px-1 text-sm font-medium">Core</legend>
            {catalog.core.map((c) => (
              <label key={c.name} className="mt-1 flex items-center gap-2 text-sm">
                <input type="checkbox" name="features" value={c.name} checked={features.has(c.name)} onChange={(e) => toggleFeature(undefined, c.name, e.target.checked)} className="size-4" /> {c.label}
              </label>
            ))}
          </fieldset>
          {catalog.apps.map((app) => (
            <fieldset key={app.name} className="rounded-xl border p-4">
              <legend className="px-1">
                <label className="flex items-center gap-2 text-sm font-medium">
                  <input type="checkbox" name="apps" value={app.name} checked={apps.has(app.name)} onChange={(e) => toggleApp(app, e.target.checked)} className="size-4" /> {app.label}
                </label>
              </legend>
              {app.features.map((x) => (
                <label key={x.name} className="mt-1 flex items-center gap-2 pl-6 text-sm">
                  <input type="checkbox" name="features" value={x.name} checked={features.has(x.name)} onChange={(e) => toggleFeature(app, x.name, e.target.checked)} className="size-4" /> {x.label}
                </label>
              ))}
            </fieldset>
          ))}
        </div>
      </Card>

      <Card className="p-5">
        <h2 className="font-medium">Words for records</h2>
        <p className="mt-1 text-xs text-muted-foreground">What this customer calls each record. Only words you change from the default are stored.</p>
        {f.labels && <p role="alert" className="mt-2 text-sm text-destructive">{f.labels}</p>}
        <div className="mt-3 grid gap-3 sm:grid-cols-2">
          {catalog.records.map((r) => (
            <div key={r.key} className="grid grid-cols-2 gap-2">
              <Input name={`label_${r.key}_singular`} aria-label={`${r.key}, singular`} value={words[r.key].singular} onChange={(e) => setWords((w) => ({ ...w, [r.key]: { ...w[r.key], singular: e.target.value } }))} maxLength={40} />
              <Input name={`label_${r.key}_plural`} aria-label={`${r.key}, plural`} value={words[r.key].plural} onChange={(e) => setWords((w) => ({ ...w, [r.key]: { ...w[r.key], plural: e.target.value } }))} maxLength={40} />
            </div>
          ))}
        </div>
      </Card>

      <div className="flex items-center gap-3">
        <SubmitButton pending="Signing…">Sign and issue</SubmitButton>
        <SavedNote state={state} />
      </div>
    </form>
  );
}

/** One business outcome. A group that is only partly on shows the dash state, and ticking it completes it. */
function GroupRow({ group, state, onChange }: { group: EntitlementGroup; state: "on" | "partly" | "off"; onChange: (on: boolean) => void }) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = state === "partly";
  }, [state]);
  return (
    <li>
      <label className="flex h-full cursor-pointer items-start gap-2.5 rounded-xl border p-3 has-[:checked]:border-primary/60 has-[:checked]:bg-primary/5">
        <input ref={ref} type="checkbox" checked={state === "on"} onChange={(e) => onChange(e.target.checked)} className="mt-1 size-4" />
        <span className="text-sm">
          <span className="font-medium">{group.label}</span>
          {state === "partly" && <span className="ml-2 text-xs text-muted-foreground">partly on</span>}
          <span className="mt-0.5 block text-xs text-muted-foreground">{group.text}</span>
        </span>
      </label>
    </li>
  );
}
