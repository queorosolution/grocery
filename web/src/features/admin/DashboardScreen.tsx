"use client";

import { useState } from "react";
import { Chips, EmptyState, MiniBarChart, Skeleton, StatTile } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { colomboDay } from "@/lib/time";
import { useAdminQuery } from "./api";
import { AdminFrame } from "./AdminFrame";
import type { AdminDashboard } from "./types";

const RANGES = [{ id: "7", label: "admin.dash.range7" }, { id: "30", label: "admin.dash.range30" }] as const;
const money = (cents: number) => formatLKR(cents, { compact: true });

function Body({ days }: { days: number }) {
  // Both ends count: "7 days" is today and the six before it.
  const from = colomboDay(-(days - 1));
  const to = colomboDay(0);
  const q = useAdminQuery<AdminDashboard>(["dashboard"], "/dashboard", { query: { from, to } });
  if (q.isPending) return <div aria-busy="true" className="grid grid-cols-2 gap-3 md:grid-cols-4">{Array.from({ length: 8 }, (_, i) => <Skeleton key={i} className="h-20" />)}</div>;
  if (q.isError) return <EmptyState title={t("common.error")} />;
  const d = q.data;
  return (
    <div className="flex flex-col gap-5">
      <section className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <StatTile label={t("admin.dash.gmv")} value={money(d.gmvCents)} />
        <StatTile label={t("admin.dash.orders")} value={d.orders} />
        <StatTile label={t("admin.dash.delivered")} value={d.delivered} />
        <StatTile label={t("admin.dash.avgBill")} value={money(d.avgBillCents)} />
        <StatTile label={t("admin.dash.income")} value={money(d.platformIncomeCents)} />
        <StatTile label={t("admin.dash.margin")} value={money(d.marginCents)} />
        <StatTile label={t("admin.dash.serviceFee")} value={money(d.serviceFeeCents)} />
        <StatTile label={t("admin.dash.couponCost")} value={money(d.couponCostCents)} />
        <StatTile label={t("admin.dash.cancelRate")} value={`${(d.cancelRateBps / 100).toFixed(1)}%`} />
        <StatTile label={t("admin.dash.failedPayments")} value={d.failedPayments} />
        <StatTile label={t("admin.dash.sms")} value={d.smsSent} />
        <StatTile label={t("admin.dash.syncIssues")} value={d.sync.openIssues} note={`${t("admin.dash.syncActive")} ${d.sync.active} · ${t("admin.dash.syncFailing")} ${d.sync.failing}`} />
      </section>
      <section aria-labelledby="chart">
        <h2 id="chart" className="mb-2 text-lg font-semibold">{t("admin.dash.chart")}</h2>
        <MiniBarChart title={t("admin.dash.chart")} bars={d.days.map((x) => ({ label: x.day.slice(5), value: x.salesCents }))} format={money} />
      </section>
      {d.shops.length > 0 && (
        <section aria-labelledby="shops">
          <h2 id="shops" className="mb-2 text-lg font-semibold">{t("admin.dash.shops")}</h2>
          <ul className="divide-y divide-line rounded-card border border-line">
            {d.shops.map((s) => (
              <li key={s.shopId} className="flex items-center justify-between gap-3 px-3 py-2">
                <span className="min-w-0 truncate">{s.name} <span className="text-muted">· {s.orders}</span></span>
                <span className="shrink-0 font-semibold">{money(s.salesCents)}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

export function AdminDashboardScreen() {
  const [days, setDays] = useState("7");
  return (
    <AdminFrame title={t("admin.section.dashboard")}>
      <div className="flex flex-col gap-4">
        <Chips label={t("admin.section.dashboard")} items={RANGES.map((r) => ({ id: r.id, label: t(r.label) }))} value={days} onChange={(v) => setDays(v ?? "7")} />
        <Body days={Number(days)} />
      </div>
    </AdminFrame>
  );
}
