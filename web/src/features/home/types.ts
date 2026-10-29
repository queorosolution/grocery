export interface ShopCardData {
  id: string;
  slug: string;
  name: string;
  logoUrl: string | null;
  themeColor: string | null;
  openNow: boolean;
  nextSlot?: string;
  distanceKm: number | null;
}

export interface Deal {
  shopProductId: string;
  shopSlug: string;
  shopName: string;
  name: string;
  imageUrl: string | null;
  onlinePriceCents: number;
  mrpCents: number;
  saveCents: number;
}

export interface Promo {
  shopSlug: string;
  title: string;
  imageUrl: string | null;
}

export interface HomeData {
  shops: ShopCardData[];
  deals: Deal[];
  promos: Promo[];
}
