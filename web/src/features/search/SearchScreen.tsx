"use client";

import { useSearchParams } from "next/navigation";
import { useState } from "react";
import { Button, EmptyState, SearchBar2Way, Skeleton } from "@/components/ui";
import { useHome, usePin } from "@/features/home/api";
import { ShopTile } from "@/features/home/ShopTile";
import { cartInfo } from "@/features/shop/cartInfo";
import { useShop } from "@/features/shop/api";
import { GRID } from "@/features/shop/ProductGrid";
import { ProductCard } from "@/features/shop/ProductCard";
import { Page } from "@/features/shell/Page";
import { t } from "@/lib/i18n";
import { useDebounced } from "@/lib/useDebounced";
import { MIN_QUERY, useSearch } from "./api";
import { ResultLink } from "./ResultLink";

const TYPING_MS = 300; // docs/08 section 5 rule 7

export function SearchScreen() {
  const params = useSearchParams();
  const first = { q: params.get("q") ?? "", shop: params.get("shop") };
  const [draft, setDraft] = useState(first);
  const settled = useDebounced(draft, TYPING_MS);
  const home = useHome();
  const search = useSearch(settled.q, settled.shop, usePin());
  // Inside one shop the results can be added to the cart; across shops they open the shop.
  const shop = useShop(settled.shop);
  const info = shop.data ? cartInfo(shop.data) : null;

  const pages = search.data?.pages ?? [];
  const products = pages.flatMap((p) => p.products.data);
  const shops = pages[0]?.shops ?? [];
  const names = new Map((home.data?.shops ?? []).map((s) => [s.slug, s.name]));
  const typing = draft !== settled && draft.q.trim().length >= MIN_QUERY;

  return (
    <Page className="flex flex-col gap-5">
      <SearchBar2Way shops={home.data?.shops ?? []} initialQuery={first.q} initialShop={first.shop}
        onChange={(q, s) => setDraft({ q, shop: s })} onSearch={(q, s) => setDraft({ q, shop: s })} />

      {settled.q.trim().length < MIN_QUERY ? (
        <EmptyState title={t("search.startTitle")} body={t("search.startBody", { min: MIN_QUERY })} />
      ) : search.isError ? (
        <EmptyState title={t("common.error")} action={<Button variant="secondary" onClick={() => void search.refetch()}>{t("common.retry")}</Button>} />
      ) : search.isPending || typing ? (
        <div className={GRID} aria-busy="true">{Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className="h-56" />)}</div>
      ) : products.length === 0 && shops.length === 0 ? (
        <EmptyState title={t("search.noneTitle")} body={t("search.noneBody")} />
      ) : (
        <>
          {shops.length > 0 && (
            <section aria-labelledby="found-shops">
              <h2 id="found-shops" className="mb-2 text-lg font-semibold">{t("search.shops")}</h2>
              <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">{shops.map((s) => <ShopTile key={s.id} shop={s} />)}</div>
            </section>
          )}
          {products.length > 0 && (
            <section aria-labelledby="found-products">
              <h2 id="found-products" className="mb-2 text-lg font-semibold">{t("search.products")}</h2>
              <div className={GRID}>
                {products.map((p) =>
                  info ? (
                    <ProductCard key={p.shopProductId} product={p} shop={info} />
                  ) : (
                    <ResultLink key={p.shopProductId} product={p} shopName={names.get(p.shopSlug ?? "")} />
                  ),
                )}
              </div>
              {search.hasNextPage && (
                <div className="mt-4 flex justify-center">
                  <Button variant="secondary" loading={search.isFetchingNextPage} onClick={() => void search.fetchNextPage()}>{t("shop.more")}</Button>
                </div>
              )}
            </section>
          )}
        </>
      )}
    </Page>
  );
}
