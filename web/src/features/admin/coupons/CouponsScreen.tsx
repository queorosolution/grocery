"use client";

import { useState } from "react";
import { Badge, Button, Chips, DataTable, EmptyState, Select, type Column } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { formatDateTime } from "@/lib/time";
import { AdminFrame } from "../AdminFrame";
import { bpToText } from "../form";
import { useShops } from "../shops/shopApi";
import type { Coupon } from "../types";
import { useCoupons } from "./api";
import { CouponDialog } from "./CouponDialog";

/** PERCENT values are basis points: 1250 is "12.5%". */
export const valueText = (c: Pick<Coupon, "type" | "value">) => (c.type === "PERCENT" ? `${bpToText(c.value)}%` : formatLKR(c.value));

export function CouponsScreen() {
  const [status, setStatus] = useState<Coupon["status"] | null>(null);
  const [shopId, setShopId] = useState("");
  // `false` = closed, `null` = making one, a coupon = changing it.
  const [editing, setEditing] = useState<Coupon | null | false>(false);
  const shops = useShops();
  const coupons = useCoupons(status, shopId || null);
  const names = new Map(shops.data?.data.map((s) => [s.id, s.name]));
  const columns: Column<Coupon>[] = [
    { key: "code", header: t("admin.coupons.code"), cell: (c) => <span className="font-semibold">{c.code}</span> },
    { key: "value", header: t("admin.coupons.value"), cell: valueText },
    { key: "shop", header: t("admin.coupons.shop"), cell: (c) => (c.shopId ? names.get(c.shopId) ?? "–" : t("admin.issues.allShops")), hideOnMobile: true },
    { key: "used", header: t("admin.coupons.used"), cell: (c) => `${c.usedCount}${c.usageLimit !== null ? ` / ${c.usageLimit}` : ""}`, hideOnMobile: true },
    { key: "ends", header: t("admin.coupons.ends"), cell: (c) => formatDateTime(c.endsAt), hideOnMobile: true },
    { key: "status", header: t("admin.shops.status"), cell: (c) => <Badge tone={c.status === "ACTIVE" ? "success" : "neutral"}>{t(`admin.coupons.status_${c.status}`)}</Badge> },
  ];
  return (
    <AdminFrame title={t("admin.section.coupons")}>
      <div className="mb-3 flex flex-col gap-2 md:flex-row md:items-center">
        <Chips label={t("admin.section.coupons")} value={status} onChange={(v) => setStatus(v as Coupon["status"] | null)}
          items={[{ id: "ACTIVE", label: t("admin.coupons.status_ACTIVE") }, { id: "PAUSED", label: t("admin.coupons.status_PAUSED") }]} />
        <Select aria-label={t("admin.coupons.shop")} className="md:w-60" value={shopId} onChange={(e) => setShopId(e.target.value)}>
          <option value="">{t("admin.issues.allShops")}</option>
          {shops.data?.data.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
        </Select>
        <span className="flex-1" />
        <Button onClick={() => setEditing(null)}>{t("admin.coupons.new")}</Button>
      </div>
      {coupons.isError ? <EmptyState title={t("common.error")} /> : (
        <DataTable caption={t("admin.section.coupons")} columns={columns} rows={coupons.data?.data ?? []} rowKey={(c) => c.id} loading={coupons.isPending}
          emptyTitle={t("admin.coupons.empty")} onRowClick={(c) => setEditing(c)} />
      )}
      <CouponDialog key={editing ? editing.id : "new"} coupon={editing || null} shops={shops.data?.data ?? []} open={editing !== false} onClose={() => setEditing(false)} />
    </AdminFrame>
  );
}
