"use client";

import { useState } from "react";
import { Chips, EmptyState, MiniBarChart, Skeleton, StatTile } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { colomboDay } from "@/lib/time";
import { useRouteId } from "@/lib/useRouteId";
import { useDashboard } from "./statsApi";
import { ShopFrame } from "./ShopFrame";

const RANGES = [{ id: "7", label: "shopConsole.range7" }, { id: "30", label: "shopConsole.range30" }] as const;
const minutes = (seconds: number) => t("shopConsole.minutes", { n: Math.round(seconds / 60) });

function Tiles({ shopId, days }: { shopId: string; days: number }) {
  // Both ends are included, so "7 days" is today and the six before it.
  const to = colomboDay(0);
  const q = useDashboard(shopId, colomboDay(-(days - 1)), to);
  if (q.isPending) return <div className="grid grid-cols-2 gap-3 md:grid-cols-4" aria-busy="true">{Array.from({ length: 8 }, (_, i) => <Skeleton key={i} className="h-20" />)}</div>;
  if (q.isError) return <EmptyState title={t("common.error")} />;
  const { queue, full } = q.data;
  const open = (...s: string[]) => s.reduce((n, k) => n + ((queue as Record<string, number>)[k] ?? 0), 0);
  return (
    <div className="flex flex-col gap-5">
      <section aria-labelledby="now" className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <h2 id="now" className="sr-only">{t("shopConsole.queue")}</h2>
        <StatTile label={t("shopConsole.tileNew")} value={open("SUBMITTED")} />
        <StatTile label={t("shopConsole.tileWaiting")} value={open("REVISED", "AWAITING_PAYMENT")} />
        <StatTile label={t("shopConsole.tilePick")} value={open("PAID", "PICKING", "PACKED")} />
        <StatTile label={t("shopConsole.tileOut")} value={open("OUT_FOR_DELIVERY", "DELIVERY_FAILED")} />
        <StatTile label={t("shopConsole.avgReview")} value={minutes(q.data.avgReviewSeconds)} />
        <StatTile label={t("shopConsole.avgPick")} value={minutes(q.data.avgPickSeconds)} />
      </section>
      {full && (
        <>
          <section className="grid grid-cols-2 gap-3 md:grid-cols-4">
            <StatTile label={t("shopConsole.tileSales")} value={formatLKR(full.salesCents, { compact: true })} />
            <StatTile label={t("shopConsole.tileOrders")} value={full.orders} />
            <StatTile label={t("shopConsole.tileAvgBill")} value={formatLKR(full.avgBillCents, { compact: true })} />
            <StatTile label={t("shopConsole.tileCancel")} value={`${(full.cancelRateBps / 100).toFixed(1)}%`} />
            <StatTile label={t("shopConsole.tileShopItems")} value={formatLKR(full.shopItemsCents, { compact: true })} />
            <StatTile label={t("shopConsole.tileDue")} value={formatLKR(full.payoutDueCents, { compact: true })} />
          </section>
          <section aria-labelledby="chart">
            <h2 id="chart" className="mb-2 text-lg font-semibold">{t("shopConsole.salesChart")}</h2>
            <MiniBarChart title={t("shopConsole.salesChart")} bars={full.days.map((d) => ({ label: d.day.slice(5), value: d.salesCents }))} format={(v) => formatLKR(v, { compact: true })} />
          </section>
          {full.topProducts.length > 0 && (
            <section aria-labelledby="top">
              <h2 id="top" className="mb-2 text-lg font-semibold">{t("shopConsole.topProducts")}</h2>
              <ul className="divide-y divide-line rounded-card border border-line">
                {full.topProducts.map((p) => (
                  <li key={p.productId} className="flex items-center justify-between gap-3 px-3 py-2">
                    <span className="min-w-0 truncate">{p.name} <span className="text-muted">× {p.quantity}</span></span>
                    <span className="shrink-0 font-semibold">{formatLKR(p.salesCents, { compact: true })}</span>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </>
      )}
    </div>
  );
}

export function DashboardScreen() {
  const shopId = useRouteId("/console/shop/:shopId/dashboard", "shopId");
  const [days, setDays] = useState("7");
  return (
    <ShopFrame shopId={shopId}>
      {(id) => (
        <div className="flex flex-col gap-4">
          <Chips label={t("shopConsole.dashboard")} items={RANGES.map((r) => ({ id: r.id, label: t(r.label) }))} value={days} onChange={(v) => setDays(v ?? "7")} />
          <Tiles shopId={id} days={Number(days)} />
        </div>
      )}
    </ShopFrame>
  );
}
