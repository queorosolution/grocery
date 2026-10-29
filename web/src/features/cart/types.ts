import type { CartLine } from "@/lib/cartApi";
import type { CartRef } from "@/lib/cartStore";

/** What the cart needs to know about the shop a product is added from (from GET /shops/{slug}). */
export interface ShopCartInfo {
  shopId: string;
  slug: string;
  name: string;
  tenantId: string;
  publicKey: string;
}

export interface CartState {
  ref: CartRef | null;
  /** Lines with the quantity the person sees: taps not yet sent are already counted. */
  lines: CartLine[];
  count: number;
  /** Estimate in cents, from cart-service prices. The bill uses our own. */
  subtotalCents: number;
  qtyOf: (itemId: string) => number;
  setQty: (shop: ShopCartInfo, itemId: string, qty: number) => void;
  clear: () => Promise<void>;
  open: boolean;
  setOpen: (open: boolean) => void;
}
