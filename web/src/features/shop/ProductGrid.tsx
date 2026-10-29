"use client";

import { Button, EmptyState, Skeleton } from "@/components/ui";
import type { ShopCartInfo } from "@/features/cart/types";
import { t } from "@/lib/i18n";
import { ProductCard } from "./ProductCard";
import type { ProductCardData } from "./types";

/** Two columns on a phone, more as the screen grows. */
export const GRID = "grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5";

interface Props {
  products: ProductCardData[];
  shop: ShopCartInfo | null;
  loading?: boolean;
  hasMore?: boolean;
  loadingMore?: boolean;
  onMore?: () => void;
}

export function ProductGrid({ products, shop, loading, hasMore, loadingMore, onMore }: Props) {
  if (loading) {
    return (
      <div className={GRID} aria-busy="true">
        {Array.from({ length: 8 }, (_, i) => <Skeleton key={i} className="h-56" />)}
      </div>
    );
  }
  if (products.length === 0) return <EmptyState title={t("shop.noProducts")} body={t("shop.noProductsBody")} />;
  return (
    <>
      <div className={GRID}>
        {products.map((p) => <ProductCard key={p.shopProductId} product={p} shop={shop} />)}
      </div>
      {hasMore && (
        <div className="mt-4 flex justify-center">
          <Button variant="secondary" loading={loadingMore} onClick={onMore}>{t("shop.more")}</Button>
        </div>
      )}
    </>
  );
}
