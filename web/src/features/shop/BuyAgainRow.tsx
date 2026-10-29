"use client";

import type { ShopCartInfo } from "@/features/cart/types";
import { t } from "@/lib/i18n";
import { ProductCard } from "./ProductCard";
import type { ProductCardData } from "./types";

/** Products the person bought here before, with today's price. Shown only when there are some. */
export function BuyAgainRow({ products, shop }: { products: ProductCardData[]; shop: ShopCartInfo | null }) {
  if (products.length === 0) return null;
  return (
    <section aria-labelledby="buy-again" className="mb-5">
      <h2 id="buy-again" className="mb-2 text-lg font-semibold">{t("shop.buyAgain")}</h2>
      <div className="-mx-4 flex gap-3 overflow-x-auto px-4 pb-1 [scrollbar-width:none]">
        {products.map((p) => (
          <div key={p.shopProductId} className="w-40 shrink-0"><ProductCard product={p} shop={shop} /></div>
        ))}
      </div>
    </section>
  );
}
