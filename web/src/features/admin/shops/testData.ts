import { adminShop as shop, superAdmin } from "@/test/fixtures";
import { bootstrap, me } from "@/test/support";
import type { PaymentAccount, ShopDetail } from "../types";

// Data for the shop setup tests.
export const detail = (over: Partial<ShopDetail> = {}): ShopDetail => ({
  shop: shop({ acceptingOrders: false }),
  settings: {
    deliveryType: "SLOT", paymentProvider: "PAYHERE", paymentAccountId: null, driverSource: "SHOP", notifyChannels: ["SMS"], markupBp: 500,
    minPlatformMarginCents: 5000, minOrderCents: 150000, maxLines: 60, reviewTimeoutMin: 30, revisionTimeoutMin: 30, paymentTimeoutMin: 60,
    cartTenantId: null, cartPublicKey: null,
  },
  hours: [{ weekday: 1, opens: "08:00", closes: "21:00" }],
  readiness: { ready: false, missing: ["PAYMENT_ACCOUNT", "FUTURE_SLOT"] },
  ...over,
});
export const account = (over: Partial<PaymentAccount> = {}): PaymentAccount => ({
  id: "a1", ownerType: "SHOP", shopId: "s1", provider: "PAYHERE", mode: "SANDBOX", merchantId: "121XXXX", status: "ACTIVE", hasSecret: true, bank: null, ...over,
});
export const base = (d = detail()) => ({
  "GET /bootstrap": () => bootstrap(me({ roles: superAdmin() })),
  "GET /admin/shops/s1": () => d,
  "GET /admin/payment-accounts": () => ({ data: [account()] }),
});
