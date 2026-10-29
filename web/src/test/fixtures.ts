import type { OrderDetail } from "@/features/orders/types";
import type { ShopDetail, ProductCardData } from "@/features/shop/types";

export const krishna: ShopDetail = {
  id: "s1", slug: "krishna-super", name: "Krishna Super", logoUrl: "https://img.example.com/k.png", themeColor: "#0F5132",
  openNow: true, acceptingOrders: true, hoursToday: { opens: "08:00", closes: "21:00" }, deliveryType: "SLOT", minOrderCents: 100000,
  cart: { tenantId: "ten_k", publicKey: "pk_k" },
  categories: [
    { id: "c1", parentId: null, slug: "rice", name: "Rice", count: 2 },
    { id: "c2", parentId: null, slug: "dairy", name: "Dairy", count: 1 },
    { id: "c3", parentId: "c1", slug: "basmati", name: "Basmati", count: 1 },
  ],
};

export const otherShop: ShopDetail = { ...krishna, id: "s2", slug: "ceylon-mart", name: "Ceylon Mart", cart: { tenantId: "ten_c", publicKey: "pk_c" } };

export const samba: ProductCardData = {
  shopProductId: "p1", productId: "g1", name: "Samba Rice", brand: "Araliya", sizeLabel: "5kg", unit: "PCS", imageUrl: null,
  onlinePriceCents: 143500, mrpCents: 150000, available: true, maxPerOrder: 20,
};

export const milk: ProductCardData = { ...samba, shopProductId: "p2", productId: "g2", name: "Fresh Milk", brand: null, sizeLabel: "1L", onlinePriceCents: 38000, mrpCents: null, available: false };

export const order = (over: Partial<OrderDetail> = {}): OrderDetail => ({
  id: "o1", code: "KS-7Q2M9X", status: "AWAITING_PAYMENT", version: 4, revisionNo: 1,
  shop: { id: "s1", slug: "krishna-super", name: "Krishna Super", phone: "+94110000000" },
  slot: { startsAt: "2026-11-02T03:00:00Z", endsAt: "2026-11-02T05:00:00Z" },
  address: { label: "Home", line1: "12 Galle Road", landmark: null },
  lines: [{ id: "l1", name: "Samba Rice 5kg", unit: "PCS", qty: 1, unitPriceCents: 143500, lineCents: 143500, state: "ACTIVE", subPref: "SIMILAR", revisionNo: 1, replacesItemId: null }],
  previousLines: [],
  totals: { itemsCents: 143500, deliveryFeeCents: 15000, serviceFeeCents: 0, discountCents: 0, totalCents: 158500 },
  coupon: null, payment: { provider: "PAYHERE", status: "INITIATED" }, deliveryCode: null,
  timeline: [{ status: "SUBMITTED", at: "2026-11-01T04:00:00Z" }, { status: "AWAITING_PAYMENT", at: "2026-11-01T04:10:00Z" }],
  allowedActions: ["PAY", "APPLY_COUPON", "CANCEL"],
  ...over,
});
