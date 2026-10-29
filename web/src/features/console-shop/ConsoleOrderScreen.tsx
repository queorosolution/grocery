"use client";

import { Badge, EmptyState, Skeleton } from "@/components/ui";
import { statusTone } from "@/components/ui/statusTone";
import { OrderLines } from "@/features/orders/OrderLines";
import { statusText } from "@/features/orders/statusText";
import { Timeline } from "@/features/orders/Timeline";
import { ApiError } from "@/lib/api";
import { t } from "@/lib/i18n";
import { formatDateTime } from "@/lib/time";
import { useRouteParams } from "@/lib/useRouteId";
import { useShopOrder } from "./api";
import { FulfilBar } from "./FulfilBar";
import { OrderSummary } from "./OrderSummary";
import { PickList } from "./PickList";
import { ReviewPanel } from "./ReviewPanel";
import { ShopFrame } from "./ShopFrame";

function Body({ shopId, orderId }: { shopId: string; orderId: string | null }) {
  const q = useShopOrder(shopId, orderId);
  if (q.isError) {
    const gone = q.error instanceof ApiError && q.error.status === 404;
    return <EmptyState title={gone ? t("order.notFoundTitle") : t("common.error")} />;
  }
  const order = q.data;
  if (!order) return <div aria-busy="true"><Skeleton className="h-8 w-48" /><Skeleton className="mt-4 h-48 w-full" /></div>;
  return (
    <div className="mx-auto flex max-w-xl flex-col gap-4">
      <header>
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-xl font-semibold">{order.code}</h2>
          <Badge tone={statusTone(order.status)}>{t(statusText[order.status])}</Badge>
        </div>
        {order.expiresAt && <p className="text-sm text-warning">{t("order.expires", { time: formatDateTime(order.expiresAt) })}</p>}
      </header>
      <ReviewPanel shopId={shopId} order={order} />
      <FulfilBar shopId={shopId} order={order} />
      {order.status === "PICKING" && <PickList lines={order.lines} />}
      <OrderSummary order={order} />
      <OrderLines order={order} />
      <Timeline steps={order.timeline} />
    </div>
  );
}

/** /console/shop/:shopId/order/:orderId. Which buttons show is decided by the server's allowedActions. */
export function ConsoleOrderScreen() {
  const ids = useRouteParams("/console/shop/:shopId/order/:orderId");
  return <ShopFrame shopId={ids?.shopId ?? null}>{(shopId) => <Body shopId={shopId} orderId={ids?.orderId ?? null} />}</ShopFrame>;
}
