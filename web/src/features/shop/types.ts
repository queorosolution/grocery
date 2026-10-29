export interface Category {
  id: string;
  parentId: string | null;
  slug: string;
  name: string;
  count: number;
}

/** GET /shops/{slug} (docs/06 section 3). */
export interface ShopDetail {
  id: string;
  slug: string;
  name: string;
  logoUrl: string | null;
  themeColor: string | null;
  openNow: boolean;
  acceptingOrders: boolean;
  hoursToday: { opens: string; closes: string } | null;
  deliveryType: "SLOT" | "COURIER";
  minOrderCents: number;
  cart: { tenantId: string; publicKey: string } | null;
  categories: Category[];
}

/** One product as the shop list, buy-again and deals show it. */
export interface ProductCardData {
  shopProductId: string;
  productId: string;
  name: string;
  brand: string | null;
  sizeLabel: string | null;
  unit: string;
  imageUrl: string | null;
  onlinePriceCents: number;
  mrpCents: number | null;
  available: boolean;
  maxPerOrder: number;
}

export interface Page<T> {
  data: T[];
  nextCursor: string | null;
}
