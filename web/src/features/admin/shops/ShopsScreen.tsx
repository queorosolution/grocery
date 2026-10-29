"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Badge, Button, DataTable, EmptyState, type Column } from "@/components/ui";
import { t, type MessageKey } from "@/lib/i18n";
import { AdminFrame } from "../AdminFrame";
import type { Shop } from "../types";
import { NewShopDialog } from "./NewShopDialog";
import { useShops } from "./shopApi";

export const statusText: Record<Shop["status"], MessageKey> = {
  DRAFT: "admin.shop.statusDRAFT",
  ACTIVE: "admin.shop.statusACTIVE",
  SUSPENDED: "admin.shop.statusSUSPENDED",
};
const tone = { DRAFT: "neutral", ACTIVE: "success", SUSPENDED: "danger" } as const;

export function ShopsScreen() {
  const shops = useShops();
  const router = useRouter();
  const [creating, setCreating] = useState(false);
  const columns: Column<Shop>[] = [
    { key: "name", header: t("admin.shops.name"), cell: (s) => <span className="font-semibold">{s.name}</span> },
    { key: "slug", header: t("admin.shops.slug"), cell: (s) => <span className="text-muted">/s/{s.slug}</span>, hideOnMobile: true },
    { key: "status", header: t("admin.shops.status"), cell: (s) => <Badge tone={tone[s.status]}>{t(statusText[s.status])}</Badge> },
    { key: "accepting", header: t("admin.shops.accepting"), cell: (s) => (s.acceptingOrders ? t("admin.shops.open") : t("admin.shops.off")) },
  ];
  return (
    <AdminFrame title={t("admin.shops.title")}>
      <div className="mb-3"><Button onClick={() => setCreating(true)}>{t("admin.shops.new")}</Button></div>
      {shops.isError ? <EmptyState title={t("common.error")} action={<Button onClick={() => void shops.refetch()}>{t("common.retry")}</Button>} /> : (
        <DataTable caption={t("admin.shops.title")} columns={columns} rows={shops.data?.data ?? []} rowKey={(s) => s.id}
          loading={shops.isPending} emptyTitle={t("admin.shops.empty")} emptyBody={t("admin.shops.emptyBody")}
          onRowClick={(s) => router.push(`/console/admin/shop/${s.id}`)} />
      )}
      <NewShopDialog open={creating} onClose={() => setCreating(false)} />
    </AdminFrame>
  );
}
