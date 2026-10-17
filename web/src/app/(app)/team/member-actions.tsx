"use client";

import { useActionState, useTransition } from "react";
import { reinviteUser, updateUser, type InviteState } from "@/app/actions-admin";
import { Button, Select } from "@/components/ui";
import type { User } from "@/lib/types";
import { CopyLink } from "./copy-link";

export function MemberActions({ user, isSelf }: { user: User; isSelf: boolean }) {
  const [pending, start] = useTransition();
  const [invite, reinvite, inviting] = useActionState<InviteState, FormData>(reinviteUser.bind(null, user.id), {});
  return (
    <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto">
      <Select
        aria-label={`Role of ${user.name}`}
        value={user.role}
        disabled={pending || isSelf}
        onChange={(e) => start(async () => void (await updateUser(user.id, { role: e.target.value })))}
        className="h-8 w-28 text-xs"
      >
        <option value="sales">Sales</option>
        <option value="admin">Admin</option>
      </Select>
      <form action={reinvite}>
        <Button type="submit" variant="secondary" size="sm" disabled={inviting}>{user.pending_invite ? "Resend invite" : "Reset password"}</Button>
      </form>
      {!isSelf && (
        <Button type="button" variant={user.active ? "destructive" : "outline"} size="sm" disabled={pending} onClick={() => start(async () => void (await updateUser(user.id, { active: !user.active })))}>
          {user.active ? "Deactivate" : "Reactivate"}
        </Button>
      )}
      {invite.inviteUrl && (
        <div className="w-full space-y-1 text-xs text-muted-foreground">
          <p>{invite.emailSent ? "Email sent. Link:" : "Email is not set up. Send this link yourself:"}</p>
          <CopyLink path={invite.inviteUrl} />
        </div>
      )}
      {invite.error && <p role="alert" className="w-full text-xs text-destructive">{invite.error}</p>}
    </div>
  );
}
