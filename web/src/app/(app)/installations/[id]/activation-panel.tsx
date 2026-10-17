"use client";

import { useState, useTransition } from "react";
import { makeActivationCode } from "@/app/actions-entitlements";
import { CheckIcon, CopyIcon } from "@/components/icons";
import { Button, Card, Input } from "@/components/ui";
import { dateTime } from "@/lib/format";
import type { ActivationState } from "@/lib/form";
import type { Activation } from "@/lib/types";

function codeStatus(a: Activation | undefined): string {
  if (!a) return "No code has been made yet.";
  if (a.used_at) return `The last code (ending ${a.hint}) was used ${dateTime(a.used_at)}.`;
  if (a.revoked_at) return `The last code (ending ${a.hint}) was replaced by a newer one.`;
  if (new Date(a.expires_at) < new Date()) return `The last code (ending ${a.hint}) expired ${dateTime(a.expires_at)}.`;
  return `A code ending ${a.hint} is waiting; it expires ${dateTime(a.expires_at)}. It can only be shown when it is made, so make a new one if it was lost.`;
}

/** Makes the one-time code the customer types at setup. The code is shown here once and never stored. */
/** The one line a customer pastes on a fresh server. The domain is theirs, so it is asked here and put into the line. */
export function installCommand(base: string, code: string, domain: string): string {
  return `curl -fsSL ${base}/install | sudo bash -s -- --code ${code} --domain ${domain.trim() || "erp.customer.com"}`;
}

const DOMAIN = /^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:\d{1,5})?$/;

export function ActivationPanel({ installationId, activation, hasFile, activated, installUrl }: { installationId: string; activation?: Activation; hasFile: boolean; activated: boolean; installUrl?: string }) {
  const [state, setState] = useState<ActivationState>({});
  const [copied, setCopied] = useState(false);
  const [copiedCommand, setCopiedCommand] = useState(false);
  const [domain, setDomain] = useState("");
  const [pending, start] = useTransition();

  function make() {
    setCopied(false);
    start(async () => setState(await makeActivationCode(installationId)));
  }
  const domainOk = DOMAIN.test(domain.trim());
  const command = state.code && installUrl ? installCommand(installUrl, state.code, domain) : "";
  async function copyCommand() {
    try {
      await navigator.clipboard.writeText(command);
      setCopiedCommand(true);
    } catch {
      setCopiedCommand(false);
    }
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(state.code ?? "");
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  return (
    <Card className="p-5">
      <h2 className="font-medium">Activation code</h2>
      <p className="mt-1 text-xs text-muted-foreground">
        The customer enters this at setup. It works once and for 7 days, and replaces any earlier code. It carries no secret you need to send separately.
      </p>
      {state.code ? (
        <div className="mt-3 rounded-xl bg-muted p-3">
          <p className="font-mono text-base font-semibold tracking-wider break-all" data-testid="activation-code">{state.code}</p>
          <p className="mt-1 text-xs text-muted-foreground">Expires {dateTime(state.expires ?? "")}. Copy it now: it cannot be shown again.</p>
          <Button type="button" variant="secondary" size="sm" className="mt-2" onClick={() => void copy()}>
            {copied ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />} {copied ? "Copied" : "Copy code"}
          </Button>
        </div>
      ) : (
        <p className="mt-3 text-sm text-muted-foreground">{codeStatus(activation)}</p>
      )}
      {state.code && (
        <div className="mt-3 rounded-xl border p-3">
          <p className="text-sm font-medium">Install on the customer&apos;s server</p>
          {installUrl ? (
            <>
              <p className="mt-1 text-xs text-muted-foreground">
                One command on a fresh Ubuntu or Debian server (as root). It installs everything and starts the app. The customer&apos;s address must already point at that server.
              </p>
              <label htmlFor="install-domain" className="mt-2 block text-xs font-medium">Their address</label>
              <Input id="install-domain" value={domain} onChange={(e) => { setDomain(e.target.value); setCopiedCommand(false); }} placeholder="erp.customer.com" autoComplete="off" spellCheck={false} aria-invalid={domain !== "" && !domainOk} />
              {domain !== "" && !domainOk && <p role="alert" className="mt-1 text-xs text-destructive">Letters, digits, dots and dashes only, like erp.customer.com.</p>}
              <pre className="mt-2 overflow-x-auto rounded-lg bg-muted p-2.5 text-xs leading-relaxed break-all whitespace-pre-wrap" data-testid="install-command">{command}</pre>
              <Button type="button" variant="secondary" size="sm" className="mt-2" disabled={!domainOk} onClick={() => void copyCommand()}>
                {copiedCommand ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />} {copiedCommand ? "Copied" : "Copy install command"}
              </Button>
            </>
          ) : (
            <p className="mt-1 text-xs text-muted-foreground">
              Set <code>CP_PUBLIC_URL</code> and <code>CP_INSTALL_REGISTRY</code> on this control plane to get a one-line installer here. Until then, install the app by hand and enter the code at setup.
            </p>
          )}
        </div>
      )}
      {state.error && <p role="alert" className="mt-2 text-sm text-destructive">{state.error}</p>}
      {!hasFile ? (
        <p className="mt-3 text-sm text-muted-foreground">Issue the first entitlement before making a code: the installation starts from it.</p>
      ) : (
        <Button type="button" size="sm" className="mt-3" onClick={make} disabled={pending}>
          {pending ? "Making…" : activated ? "Make a new code" : "Make activation code"}
        </Button>
      )}
    </Card>
  );
}
