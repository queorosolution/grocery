// The one place query keys are written (docs/08 section 5 rule 2). Never build a
// key array inline: a typo there is a silent cache miss.

export const keys = {
  bootstrap: () => ["bootstrap"] as const,
  me: () => ["me"] as const,
  home: (lat: number | null, lng: number | null) => ["home", lat, lng] as const,
  shop: (slug: string) => ["shop", slug] as const,
  products: (slug: string, category: string | null) => ["products", slug, category] as const,
  orders: () => ["orders"] as const,
  order: (id: string) => ["order", id] as const,
};
