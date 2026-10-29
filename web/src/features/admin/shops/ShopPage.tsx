"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense } from "react";
import { Badge, Button, EmptyState, Skeleton, TabPanel, Tabs } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useRouteId } from "@/lib/useRouteId";
import { AdminFrame } from "../AdminFrame";
import { Readiness } from "./Readiness";
import { statusText } from "./ShopsScreen";
import { SHOP_TABS } from "./tabs";
import { useShopDetail } from "./shopApi";

function Setup() {
  const id = useRouteId("/console/admin/shop/:id");
  const detail = useShopDetail(id);
  const router = useRouter();
  const wanted = useSearchParams().get("tab");
  const tab = SHOP_TABS.find((x) => x.id === wanted)?.id ?? SHOP_TABS[0]!.id;
  const go = (next: string) => router.replace(`/console/admin/shop/${id}?tab=${next}`);

  if (id === null || detail.isPending) return <AdminFrame title="…"><Skeleton className="h-40" /></AdminFrame>;
  if (detail.isError) {
    const gone = detail.error instanceof ApiError && detail.error.status === 404;
    return <AdminFrame title={t("admin.shops.title")}><EmptyState title={gone ? t("admin.shop.notFound") : t("common.error")} action={gone ? undefined : <Button onClick={() => void detail.refetch()}>{t("common.retry")}</Button>} /></AdminFrame>;
  }
  const d = detail.data;
  return (
    <AdminFrame title={d.shop.name}>
      <div className="mb-3 flex items-center gap-2 text-sm text-muted">
        <Badge tone={d.shop.status === "ACTIVE" ? "success" : d.shop.status === "SUSPENDED" ? "danger" : "neutral"}>{t(statusText[d.shop.status])}</Badge>
        <span>/s/{d.shop.slug}</span>
      </div>
      <Readiness detail={d} onGo={go} />
      <Tabs idPrefix="shop" label={t("admin.shop.tabs")} value={tab} onChange={go} tabs={SHOP_TABS.map((x) => ({ id: x.id, label: t(x.label) }))} />
      {SHOP_TABS.map(({ id: tabId, Tab }) => (
        <TabPanel key={tabId} idPrefix="shop" id={tabId} active={tab === tabId}><div className="pt-4"><Tab detail={d} /></div></TabPanel>
      ))}
    </AdminFrame>
  );
}

export function ShopPage() {
  return <Suspense><Setup /></Suspense>;
}
