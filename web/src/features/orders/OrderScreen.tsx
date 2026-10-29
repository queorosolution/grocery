"use client";

import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { useRouteId } from "@/lib/useRouteId";

/** Placeholder: proves the id is read from the URL after a rewrite. */
export function OrderScreen() {
  const id = useRouteId("/order/:id");
  return (
    <Page>
      <h1 className="text-xl font-semibold md:text-2xl">{t("order.title")}</h1>
      <p className="mt-1 text-muted">
        {t("order.reference")}: <span data-testid="order-id">{id ?? "…"}</span>
      </p>
      <p className="mt-4">{t("order.soon")}</p>
    </Page>
  );
}
