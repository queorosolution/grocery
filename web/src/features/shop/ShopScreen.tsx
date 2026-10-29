"use client";

import { useState } from "react";
import { Chips, EmptyState, Skeleton } from "@/components/ui";
import { useMe } from "@/features/auth/api";
import { Page } from "@/features/shell/Page";
import { ApiError } from "@/lib/api";
import { t } from "@/lib/i18n";
import { useRouteId } from "@/lib/useRouteId";
import { useBuyAgain, useProducts, useShop } from "./api";
import { BuyAgainRow } from "./BuyAgainRow";
import { CartBar } from "./CartBar";
import { cartInfo } from "./cartInfo";
import { ProductGrid } from "./ProductGrid";
import { ShopHeader } from "./ShopHeader";

/** /s/<slug>. Also what not-found.tsx shows for a shop that is newer than the last build. */
export function ShopScreen() {
  const slug = useRouteId("/s/:slug", "slug");
  const shop = useShop(slug);
  const [category, setCategory] = useState<string | null>(null);
  const products = useProducts(slug, category);
  const { me } = useMe();
  const again = useBuyAgain(slug, me !== null && shop.isSuccess);

  if (shop.isError) {
    const gone = shop.error instanceof ApiError && shop.error.status === 404;
    return (
      <Page>
        <EmptyState title={gone ? t("shop.notFoundTitle") : t("common.error")} body={gone ? t("shop.notFoundBody") : undefined} />
      </Page>
    );
  }
  if (!shop.data) {
    return (
      <Page aria-busy="true">
        <Skeleton className="h-16 w-full" />
        <Skeleton className="mt-4 h-56 w-full" />
      </Page>
    );
  }

  const info = cartInfo(shop.data);
  const top = shop.data.categories.filter((c) => c.parentId === null);
  const items = [{ id: "all", label: t("shop.all") }, ...top.map((c) => ({ id: c.slug, label: c.name }))];
  const rows = products.data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <>
      <ShopHeader shop={shop.data} />
      <Page className="min-h-[60dvh]">
        {top.length > 0 && (
          <Chips className="mb-4" label={t("shop.categories")} items={items} value={category ?? "all"}
            onChange={(id) => setCategory(id === null || id === "all" ? null : id)} />
        )}
        {category === null && <BuyAgainRow products={again.data?.data ?? []} shop={info} />}
        <ProductGrid products={rows} shop={info} loading={products.isPending} hasMore={products.hasNextPage}
          loadingMore={products.isFetchingNextPage} onMore={() => void products.fetchNextPage()} />
      </Page>
      <CartBar />
    </>
  );
}
