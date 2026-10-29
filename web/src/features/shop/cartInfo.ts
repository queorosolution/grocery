import type { ShopCartInfo } from "@/features/cart/types";
import type { ShopDetail } from "./types";

/** What the cart needs from a shop. Null while the shop has no cart tenant. */
export function cartInfo(shop: ShopDetail): ShopCartInfo | null {
  return shop.cart
    ? { shopId: shop.id, slug: shop.slug, name: shop.name, tenantId: shop.cart.tenantId, publicKey: shop.cart.publicKey }
    : null;
}
