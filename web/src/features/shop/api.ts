import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys } from "@/lib/keys";
import type { Page, ProductCardData, ShopDetail } from "./types";

const MINUTE = 60_000;
const path = (slug: string) => `/shops/${encodeURIComponent(slug)}`;

export function useShop(slug: string | null) {
  return useQuery({
    queryKey: keys.shop(slug ?? ""),
    queryFn: ({ signal }) => api<ShopDetail>(path(slug!), { signal }),
    enabled: slug !== null,
    staleTime: MINUTE,
  });
}

/** The shop's products, one category at a time, 24 per page (docs/06 section 3). */
export function useProducts(slug: string | null, category: string | null) {
  return useInfiniteQuery({
    queryKey: keys.products(slug ?? "", category),
    initialPageParam: null as string | null,
    queryFn: ({ pageParam, signal }) =>
      api<Page<ProductCardData>>(`${path(slug!)}/products`, { query: { category, cursor: pageParam }, signal }),
    getNextPageParam: (last) => last.nextCursor,
    enabled: slug !== null,
    staleTime: MINUTE,
  });
}

/** What the signed-in person bought here before. Never cached by the server, so always asked. */
export function useBuyAgain(slug: string | null, signedIn: boolean) {
  return useQuery({
    queryKey: keys.buyAgain(slug ?? ""),
    queryFn: ({ signal }) => api<{ data: ProductCardData[] }>(`${path(slug!)}/buy-again`, { signal }),
    enabled: slug !== null && signedIn,
    staleTime: MINUTE,
  });
}
