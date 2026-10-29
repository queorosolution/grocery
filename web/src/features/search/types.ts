import type { ShopCardData } from "@/features/home/types";
import type { Page, ProductCardData } from "@/features/shop/types";

/** A product from a search over many shops: it also says which shop sells it. */
export type SearchProduct = ProductCardData & { shopSlug?: string };

export interface SearchResult {
  shops: ShopCardData[];
  products: Page<SearchProduct>;
}
