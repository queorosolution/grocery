"use client";

import { Badge, DataTable, StatTile, type Column } from "@/components/ui";
import { useMe } from "@/features/auth/api";
import { shopPermissions } from "@/features/auth/roles";
import { EmptyState } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { formatDay } from "@/lib/time";
import { useRouteId } from "@/lib/useRouteId";
import { ShopFrame } from "./ShopFrame";
import { usePayouts } from "./statsApi";
import type { Payout } from "./types";

const columns: Column<Payout>[] = [
  { key: "period", header: t("shopConsole.colPeriod"), cell: (p) => `${formatDay(p.periodFrom)} – ${formatDay(p.periodTo)}` },
  { key: "amount", header: t("shopConsole.colAmount"), align: "right", cell: (p) => formatLKR(p.amountCents) },
  { key: "status", header: t("shopConsole.colStatus"), cell: (p) => <Badge tone={p.status === "PAID" ? "success" : "warning"}>{t(p.status === "PAID" ? "shopConsole.statusPaid" : "shopConsole.statusDraft")}</Badge> },
  { key: "ref", header: t("shopConsole.colRef"), hideOnMobile: true, cell: (p) => p.reference ?? "–" },
];

function Payouts({ shopId }: { shopId: string }) {
  const { me } = useMe();
  const allowed = shopPermissions(me, shopId).has("dashboard.shop_full");
  const q = usePayouts(shopId, allowed);
  if (!allowed) return <EmptyState title={t("guard.deniedTitle")} body={t("guard.deniedBody")} />;
  return (
    <div className="flex max-w-2xl flex-col gap-4">
      <StatTile label={t("shopConsole.balance")} value={formatLKR(q.data?.balanceCents ?? 0)} />
      <DataTable caption={t("shopConsole.payouts")} columns={columns} rows={q.data?.data ?? []} rowKey={(p) => p.id} loading={q.isPending} emptyTitle={t("shopConsole.payoutsEmpty")} />
    </div>
  );
}

export function PayoutsScreen() {
  const shopId = useRouteId("/console/shop/:shopId/payouts", "shopId");
  return <ShopFrame shopId={shopId}>{(id) => <Payouts shopId={id} />}</ShopFrame>;
}
