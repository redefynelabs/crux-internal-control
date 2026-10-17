"use client";

import { useState, useSyncExternalStore } from "react";
import { CheckIcon, CopyIcon } from "@/components/icons";
import { Button } from "@/components/ui";

/** An invite link (a path from the API made absolute in the browser) with a copy button. */
export function CopyLink({ path }: { path: string }) {
  const origin = useSyncExternalStore(() => () => {}, () => window.location.origin, () => "");
  const [copied, setCopied] = useState(false);
  const url = origin + path;
  return (
    <div className="flex items-center gap-2">
      <input readOnly value={url} aria-label="Invite link" onFocus={(e) => e.currentTarget.select()} className="h-9 min-w-0 flex-1 rounded-4xl border border-input bg-input/30 px-3.5 text-xs outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50" />
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(url);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
          } catch {
            /* the field is selectable by hand */
          }
        }}
      >
        {copied ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />} {copied ? "Copied" : "Copy"}
      </Button>
    </div>
  );
}
