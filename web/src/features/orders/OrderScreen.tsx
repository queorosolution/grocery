"use client";

import { Badge, EmptyState, Skeleton } from "@/components/ui";
import { RequireUser } from "@/features/auth/guards";
import { Page } from "@/features/shell/Page";
import { ApiError } from "@/lib/api";
import { t } from "@/lib/i18n";
import { formatDateTime, formatSlot } from "@/lib/time";
import { statusTone } from "@/components/ui/statusTone";
import { useRouteId } from "@/lib/useRouteId";
import { useOrder } from "./api";
import { ActionBar } from "./ActionBar";
import { CouponBox } from "./CouponBox";
import { DeliveryCodeCard } from "./DeliveryCodeCard";
import { OrderLines } from "./OrderLines";
import { PayButton } from "./PayButton";
import { RevisionCompare } from "./RevisionCompare";
import { statusText } from "./statusText";
import { Timeline } from "./Timeline";

function Body({ id }: { id: string | null }) {
  const q = useOrder(id);
  if (q.isError) {
    const gone = q.error instanceof ApiError && q.error.status === 404;
    return <EmptyState title={gone ? t("order.notFoundTitle") : t("common.error")} body={gone ? t("order.notFoundBody") : undefined} />;
  }
  const order = q.data;
  if (!order) return <div aria-busy="true"><Skeleton className="h-8 w-48" /><Skeleton className="mt-4 h-48 w-full" /></div>;

  return (
    <div className="mx-auto flex max-w-xl flex-col gap-4">
      <header>
        <div className="flex items-center justify-between gap-2">
          <h1 className="text-xl font-semibold">{order.code}</h1>
          <Badge tone={statusTone(order.status)}>{t(statusText[order.status])}</Badge>
        </div>
        <p className="text-muted">{t("order.from", { shop: order.shop.name })}</p>
        {order.expiresAt && <p className="text-sm text-warning">{t("order.expires", { time: formatDateTime(order.expiresAt) })}</p>}
      </header>
      {order.deliveryCode && <DeliveryCodeCard code={order.deliveryCode} />}
      {order.previousLines.length > 0 && order.allowedActions.includes("ACCEPT_REVISION") && <RevisionCompare order={order} />}
      <OrderLines order={order} />
      <CouponBox order={order} />
      <section className="text-sm">
        <h2 className="text-base font-semibold">{t("order.deliverTo")}</h2>
        <p>{order.address.label}: {order.address.line1}{order.address.landmark ? `, ${order.address.landmark}` : ""}</p>
        {order.slot && <p className="text-muted">{t("order.slot")}: {formatSlot(order.slot.startsAt, order.slot.endsAt)}</p>}
      </section>
      <PayButton order={order} />
      <ActionBar order={order} />
      <Timeline steps={order.timeline} />
    </div>
  );
}

export function OrderScreen() {
  const id = useRouteId("/order/:id");
  return (
    <Page>
      <RequireUser><Body id={id} /></RequireUser>
    </Page>
  );
}
