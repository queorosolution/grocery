"use client";

import { ShoppingBasket } from "lucide-react";
import { useCart } from "@/features/cart/cartContext";
import { t } from "@/lib/i18n";
import { formatLKR } from "@/lib/money";

/** Sticks to the bottom of the shop page while the cart has items. Opens the cart. */
export function CartBar() {
  const { count, subtotalCents, setOpen } = useCart();
  if (count === 0) return null;
  return (
    <div className="sticky bottom-0 z-10 border-t border-line bg-bg p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
      <button type="button" onClick={() => setOpen(true)}
        className="mx-auto flex h-12 w-full max-w-[1120px] items-center justify-between rounded-input bg-accent px-4 font-semibold text-on-accent">
        <span className="inline-flex items-center gap-2">
          <ShoppingBasket size={20} aria-hidden="true" />
          {count === 1 ? t("cart.itemOne") : t("cart.items", { count })}
        </span>
        <span>{t("cart.viewCart", { amount: formatLKR(subtotalCents) })}</span>
      </button>
    </div>
  );
}
