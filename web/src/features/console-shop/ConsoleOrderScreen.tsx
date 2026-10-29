"use client";

import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { useRouteParams } from "@/lib/useRouteId";

export function ConsoleOrderScreen() {
  const ids = useRouteParams("/console/shop/:shopId/order/:orderId");
  return (
    <Page>
      <h1 className="text-xl font-semibold md:text-2xl">{t("console.orderTitle")}</h1>
      <p className="mt-1 text-muted">
        <span data-testid="console-shop-id">{ids?.shopId ?? "…"}</span>
        {" / "}
        <span data-testid="console-order-id">{ids?.orderId ?? "…"}</span>
      </p>
      <p className="mt-4">{t("console.soon")}</p>
    </Page>
  );
}
