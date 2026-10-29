// The one place query keys are written (docs/08 section 5 rule 2). Never build a
// key array inline: a typo there is a silent cache miss.

export const keys = {
  bootstrap: () => ["bootstrap"] as const,
  me: () => ["me"] as const,
  addresses: () => ["addresses"] as const,
  home: (lat: number | null, lng: number | null) => ["home", lat, lng] as const,
  shop: (slug: string) => ["shop", slug] as const,
  products: (slug: string, category: string | null) => ["products", slug, category] as const,
  search: (q: string, shop: string | null, lat: number | null, lng: number | null) => ["search", q, shop, lat, lng] as const,
  buyAgain: (slug: string) => ["buyAgain", slug] as const,
  cart: (cartId: string) => ["cart", cartId] as const,
  orders: () => ["orders"] as const,
  /** Everything under /console/shops/{shopId}: keys.shopConsole(id, "orders", tab). */
  shopConsole: (shopId: string, ...parts: readonly unknown[]) => ["shopConsole", shopId, ...parts] as const,
  quote: (...parts: readonly unknown[]) => ["quote", ...parts] as const,
  order: (id: string) => ["order", id] as const,
};
