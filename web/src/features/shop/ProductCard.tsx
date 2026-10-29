"use client";

import { Badge, Button, Price, QtyStepper, Thumb } from "@/components/ui";
import { useCart } from "@/features/cart/cartContext";
import type { ShopCartInfo } from "@/features/cart/types";
import { t } from "@/lib/i18n";
import type { ProductCardData } from "./types";

interface Props {
  product: ProductCardData;
  /** Where the cart for this product lives. Null: the shop has no cart yet, so browse only. */
  shop: ShopCartInfo | null;
}

export function ProductCard({ product: p, shop }: Props) {
  const cart = useCart();
  const qty = cart.qtyOf(p.shopProductId);
  const set = (q: number) => shop && cart.setQty(shop, p.shopProductId, q);
  const detail = [p.brand, p.sizeLabel].filter(Boolean).join(" · ");

  return (
    <article className="flex flex-col gap-2 rounded-card border border-line bg-bg p-3">
      <Thumb src={p.imageUrl} className="w-full" />
      <div className="min-h-[3.5rem] min-w-0">
        <h3 className="line-clamp-2 text-base">{p.name}</h3>
        {detail && <p className="truncate text-sm text-muted">{detail}</p>}
      </div>
      <Price cents={p.onlinePriceCents} mrpCents={p.mrpCents} showSave />
      <div className="mt-auto">
        {!p.available ? (
          <Badge>{t("product.unavailable")}</Badge>
        ) : !shop ? null : qty === 0 ? (
          <Button block variant="secondary" onClick={() => set(1)}>{t("product.add")}</Button>
        ) : (
          <QtyStepper className="w-full justify-between" value={qty} max={p.maxPerOrder} onChange={set} />
        )}
      </div>
    </article>
  );
}
