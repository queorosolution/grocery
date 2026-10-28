"use client";

import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { useRouteId } from "@/lib/useRouteId";

/** Placeholder. Also what not-found.tsx shows for a shop that is newer than the last build. */
export function ShopScreen() {
  const slug = useRouteId("/s/:slug", "slug");
  return (
    <Page>
      <h1 className="text-xl font-semibold md:text-2xl">{t("shop.title")}</h1>
      <p className="mt-1 text-muted" data-testid="shop-slug">{slug ?? t("shop.loading")}</p>
      <p className="mt-4">{t("shop.soon")}</p>
    </Page>
  );
}
