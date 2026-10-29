"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo, useState, useSyncExternalStore, type ReactNode } from "react";
import { cartApi, CartError, type Cart } from "@/lib/cartApi";
import { readCartRef, subscribeCartRef, writeCartRef } from "@/lib/cartStore";
import { keys } from "@/lib/keys";
import { CartContext } from "./cartContext";
import { CartSheet } from "./CartSheet";
import { SwitchShopDialog } from "./SwitchShopDialog";
import type { CartState, ShopCartInfo } from "./types";
import { useCartWriter } from "./useCartWriter";

const MINUTE = 60_000;

interface Wanted {
  shop: ShopCartInfo;
  itemId: string;
  qty: number;
}

/** The person's cart: which shop it belongs to, its lines, and every change to it (docs/08 section 6). */
export function CartProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const ref = useSyncExternalStore(subscribeCartRef, readCartRef, () => null);
  const [open, setOpen] = useState(false);
  const [switching, setSwitching] = useState<Wanted | null>(null);
  const { pending, setQty: write, reset } = useCartWriter();

  const query = useQuery({
    queryKey: keys.cart(ref?.cartId ?? "none"),
    enabled: ref !== null,
    staleTime: MINUTE,
    queryFn: async (): Promise<Cart | null> => {
      try {
        return await cartApi.get(ref!.publicKey, ref!.cartId);
      } catch (err) {
        if (err instanceof CartError && err.status === 404) {
          writeCartRef(null); // an expired cart is no cart
          return null;
        }
        throw err;
      }
    },
  });

  const view = useMemo(() => {
    const lines = (query.data?.lines ?? []).map((l) => ({ ...l, quantity: pending[l.itemId ?? ""] ?? l.quantity }));
    const known = new Set(lines.map((l) => l.itemId));
    const fresh = Object.entries(pending).filter(([id]) => !known.has(id)).reduce((n, [, q]) => n + q, 0);
    const shown = lines.filter((l) => l.quantity > 0);
    return {
      lines: shown,
      count: shown.reduce((n, l) => n + l.quantity, 0) + fresh,
      subtotalCents: shown.reduce((n, l) => n + l.quantity * l.unitPrice, 0),
    };
  }, [query.data, pending]);

  const qtyOf = useCallback(
    (itemId: string) => pending[itemId] ?? query.data?.lines.find((l) => l.itemId === itemId)?.quantity ?? 0,
    [pending, query.data],
  );

  const setQty = useCallback(
    (shop: ShopCartInfo, itemId: string, qty: number) => {
      // Items of another shop in the cart: ask before throwing them away.
      if (ref && ref.shopSlug !== shop.slug && view.count > 0) setSwitching({ shop, itemId, qty });
      else write(shop, itemId, qty);
    },
    [ref, view.count, write],
  );

  const clear = useCallback(async () => {
    const old = readCartRef();
    reset();
    writeCartRef(null);
    if (old) {
      client.removeQueries({ queryKey: keys.cart(old.cartId) });
      await cartApi.discard(old.publicKey, old.cartId).catch(() => undefined); // it expires by itself anyway
    }
  }, [client, reset]);

  const state = useMemo<CartState>(
    () => ({ ref, ...view, qtyOf, setQty, clear, open, setOpen }),
    [ref, view, qtyOf, setQty, clear, open],
  );

  const confirmSwitch = () => {
    const next = switching;
    setSwitching(null);
    if (!next) return;
    void clear().then(() => write(next.shop, next.itemId, next.qty));
  };

  return (
    <CartContext.Provider value={state}>
      {children}
      <CartSheet />
      <SwitchShopDialog from={ref?.shopName ?? ""} to={switching?.shop.name ?? ""} open={switching !== null}
        onConfirm={confirmSwitch} onClose={() => setSwitching(null)} />
    </CartContext.Provider>
  );
}
