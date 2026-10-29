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
