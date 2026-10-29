import { useCart } from "@/features/cart/cartContext";
import type { MessageKey } from "@/lib/i18n";

/**
 * Reasons from the browser itself that block deleting the account. The cart lives in the cart
 * service, not in our API, so only the web app can see it.
 */
export function useCartReasons(): MessageKey[] {
  const { count } = useCart();
  return count > 0 ? ["account.blocked.cart"] : [];
}
