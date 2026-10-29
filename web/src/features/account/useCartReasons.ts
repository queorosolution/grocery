import type { MessageKey } from "@/lib/i18n";

/**
 * Reasons from the browser itself that block deleting the account. The cart lives in the cart
 * service, not in our API, so only the web app can see it; the cart screens (P29) return
 * "account.blocked.cart" here while the cart has items.
 */
export function useCartReasons(): MessageKey[] {
  return [];
}
