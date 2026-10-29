import type { Shop } from "@/features/admin/types";
import type { Me } from "@/features/auth/types";
import type { OrderDetail } from "@/features/orders/types";
import type { QueueRow, ShopOrder } from "@/features/console-shop/types";
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


export const owner = (): Me["roles"] => [{ role: "SHOP_OWNER", shopId: "s1", shopSlug: "krishna-super", shopName: "Krishna Super", canEditProducts: true }];
export const staff = (canEditProducts = false): Me["roles"] => [{ role: "SHOP_STAFF", shopId: "s1", shopSlug: "krishna-super", shopName: "Krishna Super", canEditProducts }];

export const queueRow = (over: Partial<QueueRow> = {}): QueueRow => ({
  id: "o1", shopId: "s1", code: "KS-7Q2M9X", status: "SUBMITTED", version: 1, customer: { name: "Nimal Perera", phoneMasked: "077•••4567" },
  slot: null, totalCents: 158500, itemCount: 2, createdAt: "2026-11-01T04:00:00Z", ...over,
});

export const shopOrder = (over: Partial<ShopOrder> = {}): ShopOrder => ({
  ...(order({ id: "o1", status: "SUBMITTED", version: 1, allowedActions: ["APPROVE", "REVISE", "REJECT"] }) as Omit<ShopOrder, "customer">),
  customer: { name: "Nimal Perera", phoneMasked: "077•••4567" },
  lines: [
    { id: "l1", name: "Samba Rice 5kg", unit: "PCS", qty: 2, unitPriceCents: 143500, lineCents: 287000, state: "ACTIVE", subPref: "SIMILAR", revisionNo: 1, replacesItemId: null },
    { id: "l2", name: "Fresh Milk 1L", unit: "PCS", qty: 1, unitPriceCents: 38000, lineCents: 38000, state: "ACTIVE", subPref: "CALL_ME", revisionNo: 1, replacesItemId: null },
  ],
  ...over,
} as ShopOrder);

/** One shop as the admin API lists it. */
export const adminShop = (over: Partial<Shop> = {}): Shop => ({
  id: "s1", slug: "krishna-super", name: "Krishna Super", phone: "+94112223344", addressText: "Main St", lat: 6.9, lng: 79.8,
  logoUrl: null, themeColor: "#1a7f4b", status: "ACTIVE", acceptingOrders: true, updatedAt: "2026-10-29T00:00:00Z", ...over,
});

export const superAdmin = (): Me["roles"] => [{ role: "SUPER_ADMIN", shopId: null, shopSlug: null, shopName: null }];
