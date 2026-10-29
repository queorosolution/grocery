"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import { useToast } from "@/components/ui";
import { cartApi, CartError, type Cart } from "@/lib/cartApi";
import { readCartRef, writeCartRef, type CartRef } from "@/lib/cartStore";
import { keys } from "@/lib/keys";
import { t } from "@/lib/i18n";
import type { ShopCartInfo } from "./types";

const DEBOUNCE_MS = 400; // docs/08 section 6: one cart call per burst of taps, per line

/** Starts a cart for `shop`. The cart of another shop is thrown away (one shop at a time). */
async function startCart(shop: ShopCartInfo): Promise<CartRef> {
  const old = readCartRef();
  if (old) void cartApi.discard(old.publicKey, old.cartId).catch(() => undefined);
  const cart = await cartApi.create(shop.publicKey);
  const ref: CartRef = {
    shopId: shop.shopId, shopSlug: shop.slug, shopName: shop.name, tenantId: shop.tenantId,
    publicKey: shop.publicKey, cartId: cart.id,
  };
  writeCartRef(ref);
  return ref;
}

/**
 * Quantity taps: shown at once (`pending`), sent 400 ms after the last tap of that line, and one
 * at a time so two calls never race on the same cart. A failed call rolls the number back.
 */
export function useCartWriter() {
  const client = useQueryClient();
  const toast = useToast();
  const [pending, setPending] = useState<Record<string, number>>({});
  const wanted = useRef(new Map<string, number>());
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const queue = useRef<Promise<void>>(Promise.resolve());

  useEffect(() => {
    const open = timers.current;
    return () => open.forEach(clearTimeout);
  }, []);

  const send = useCallback(
    async (shop: ShopCartInfo, itemId: string) => {
      const qty = wanted.current.get(itemId) ?? 0;
      try {
        let ref = readCartRef();
        if (ref?.shopSlug !== shop.slug) ref = await startCart(shop);
        const cached = client.getQueryData<Cart | null>(keys.cart(ref.cartId));
        const line = cached?.lines.find((l) => l.itemId === itemId);
        let next: Cart | null | undefined = cached;
        if (line) next = await cartApi.setQuantity(ref.publicKey, ref.cartId, line.id, qty);
        else if (qty > 0) next = await cartApi.addLine(ref.publicKey, ref.cartId, itemId, qty);
        client.setQueryData(keys.cart(ref.cartId), next);
      } catch (err) {
        if (err instanceof CartError && err.status === 404) writeCartRef(null); // the cart expired
        toast.show(t("cart.failed"), "danger");
      } finally {
        if (wanted.current.get(itemId) === qty) {
          wanted.current.delete(itemId);
          setPending((p) => Object.fromEntries(Object.entries(p).filter(([id]) => id !== itemId)));
        }
      }
    },
    [client, toast],
  );

  const setQty = useCallback(
    (shop: ShopCartInfo, itemId: string, qty: number) => {
      wanted.current.set(itemId, qty);
      setPending((p) => ({ ...p, [itemId]: qty }));
      clearTimeout(timers.current.get(itemId));
      timers.current.set(
        itemId,
        setTimeout(() => {
          timers.current.delete(itemId);
          queue.current = queue.current.then(() => send(shop, itemId));
        }, DEBOUNCE_MS),
      );
    },
    [send],
  );

  const reset = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current.clear();
    wanted.current.clear();
    setPending({});
  }, []);

  return { pending, setQty, reset };
}
