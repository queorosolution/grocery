"use client";

import Link from "next/link";
import { ShoppingBasket } from "lucide-react";
import { Button, EmptyState, Price, QtyStepper, Sheet } from "@/components/ui";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";
import { useCart } from "./cartContext";
import type { ShopCartInfo } from "./types";

/** The cart drawer: lines with steppers, the estimate, and the way to checkout. */
export function CartSheet() {
  const cart = useCart();
  const { ref, lines } = cart;
  // Taps go through the same path as the shop page; the shop is the one the cart belongs to.
  const shop: ShopCartInfo | null = ref && { shopId: ref.shopId, slug: ref.shopSlug, name: ref.shopName, tenantId: ref.tenantId, publicKey: ref.publicKey };
  const close = () => cart.setOpen(false);

  return (
    <Sheet
      open={cart.open}
      onClose={close}
      title={ref ? t("cart.titleShop", { shop: ref.shopName }) : t("cart.title")}
      footer={
        lines.length > 0 && (
          <div className="flex w-full items-center gap-3">
            <div className="min-w-0 flex-1">
              <div className="text-sm text-muted">{t("cart.estimate")}</div>
              <div className="font-semibold">{formatLKR(cart.subtotalCents)}</div>
            </div>
            <Link href="/checkout" onClick={close}
              className="inline-flex h-12 items-center rounded-input bg-accent px-6 font-semibold text-on-accent">
              {t("cart.checkout")}
            </Link>
          </div>
        )
      }
    >
      {lines.length === 0 || !shop ? (
        <EmptyState icon={<ShoppingBasket size={32} />} title={t("cart.emptyTitle")} body={t("cart.emptyBody")} />
      ) : (
        <>
          <ul className="divide-y divide-line">
            {lines.map((l) => (
              <li key={l.id} className="flex items-center gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <div className="truncate">{l.name}</div>
                  <Price cents={l.unitPrice} className="text-sm text-muted" />
                </div>
                <QtyStepper value={l.quantity} onChange={(q) => l.itemId && cart.setQty(shop, l.itemId, q)} />
              </li>
            ))}
          </ul>
          <Button variant="ghost" className="mt-2" onClick={() => void cart.clear()}>{t("cart.clear")}</Button>
        </>
      )}
    </Sheet>
  );
}
