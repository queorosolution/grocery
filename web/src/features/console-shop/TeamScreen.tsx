"use client";

import { useState } from "react";
import { Badge, Button, Checkbox, ConfirmDialog, EmptyState, Skeleton } from "@/components/ui";
import { useMe } from "@/features/auth/api";
import { shopPermissions } from "@/features/auth/roles";
import type { Role } from "@/lib/enums.gen";
import { errorText } from "@/lib/errorText";
import { t, type MessageKey } from "@/lib/i18n";
import { formatDateTime } from "@/lib/time";
import { useRouteId } from "@/lib/useRouteId";
import { InviteDialog } from "./InviteDialog";
import { ShopFrame } from "./ShopFrame";
import { usePatchMember, useRemoveMember, useTeam } from "./teamApi";
import type { Member } from "./types";

const roleText: Partial<Record<Role, MessageKey>> = { SHOP_OWNER: "shopConsole.roleOwner", SHOP_STAFF: "shopConsole.roleStaff", SHOP_DRIVER: "shopConsole.roleDriver" };

function Members({ shopId }: { shopId: string }) {
  const { me } = useMe();
  const team = useTeam(shopId);
  const patch = usePatchMember(shopId);
  const remove = useRemoveMember(shopId);
  const [inviting, setInviting] = useState(false);
  const [removing, setRemoving] = useState<Member | null>(null);

  if (team.isPending) return <Skeleton className="h-40 w-full" />;
  if (team.isError) return <EmptyState title={errorText(team.error)} />;
  const { members, invites } = team.data;
  const error = patch.error ?? remove.error;

  return (
    <div className="flex max-w-2xl flex-col gap-5">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">{t("shopConsole.teamMembers")}</h2>
        <Button onClick={() => setInviting(true)}>{t("shopConsole.inviteTitle")}</Button>
      </div>
      <ul className="divide-y divide-line rounded-card border border-line">
        {members.map((m) => {
          const mine = m.userId === me?.id;
          return (
            <li key={m.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
              <div className="min-w-0 flex-1">
                <div className="truncate font-semibold">{m.name}{mine && <span className="ml-2 text-sm font-normal text-muted">{t("shopConsole.you")}</span>}</div>
                <div className="truncate text-sm text-muted">{m.email}</div>
              </div>
              <Badge>{t(roleText[m.role] ?? "shopConsole.roleStaff")}</Badge>
              {m.role === "SHOP_STAFF" && !mine && (
                <Checkbox label={t("shopConsole.canEdit")} checked={m.canEditProducts} onChange={(v) => patch.mutate({ id: m.id, canEditProducts: v })} />
              )}
              {m.role !== "SHOP_OWNER" && !mine && <Button variant="ghost" onClick={() => setRemoving(m)}>{t("shopConsole.removeMember")}</Button>}
            </li>
          );
        })}
      </ul>
      {error && <p role="alert" className="text-sm text-danger">{errorText(error)}</p>}
      {invites.length > 0 && (
        <section aria-labelledby="invites">
          <h2 id="invites" className="mb-2 text-lg font-semibold">{t("shopConsole.teamInvites")}</h2>
          <ul className="divide-y divide-line rounded-card border border-line">
            {invites.map((i) => (
              <li key={i.id} className="flex items-center justify-between gap-2 px-3 py-2">
                <span className="truncate">{i.email}</span>
                <span className="shrink-0 text-sm text-muted">{t("shopConsole.expires", { time: formatDateTime(i.expiresAt) })}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
      <InviteDialog shopId={shopId} open={inviting} onClose={() => setInviting(false)} />
      <ConfirmDialog open={removing !== null} danger title={t("shopConsole.removeTitle", { name: removing?.name ?? "" })} message={t("shopConsole.removeBody")}
        confirmLabel={t("shopConsole.removeMember")} loading={remove.isPending} onClose={() => setRemoving(null)}
        onConfirm={() => removing && remove.mutate(removing.id, { onSettled: () => setRemoving(null) })} />
    </div>
  );
}

export function TeamScreen() {
  const shopId = useRouteId("/console/shop/:shopId/team", "shopId");
  const { me } = useMe();
  return (
    <ShopFrame shopId={shopId}>
      {(id) => shopPermissions(me, id).has("team.manage") ? <Members shopId={id} /> : <EmptyState title={t("guard.deniedTitle")} body={t("guard.deniedBody")} />}
    </ShopFrame>
  );
}
