"use client";

import { ShoppingBasket } from "lucide-react";
import { t } from "@/lib/i18n";
import { useCart } from "./cartContext";

/** The basket in the header, with the number of items on it. */
export function CartButton() {
  const { count, setOpen } = useCart();
  return (
    <button type="button" aria-label={t("cart.open", { count })} onClick={() => setOpen(true)}
      className="relative inline-flex size-11 items-center justify-center rounded-pill hover:bg-surface">
      <ShoppingBasket size={22} aria-hidden="true" />
      {count > 0 && (
        <span aria-hidden="true"
          className="absolute right-0.5 top-0.5 inline-flex min-w-5 items-center justify-center rounded-pill bg-accent px-1 text-xs font-semibold text-on-accent">
          {count}
        </span>
      )}
    </button>
  );
}
