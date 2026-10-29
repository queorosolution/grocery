import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys } from "@/lib/keys";
import type { ShopProduct } from "./types";

const base = (shopId: string) => `/console/shops/${encodeURIComponent(shopId)}/products`;

/** The shop's own products by name, 50 per page. `q` is a word of the name, brand or keywords. */
export function useShopProducts(shopId: string, q: string, available: boolean | null, enabled = true) {
  const text = q.trim();
  return useInfiniteQuery({
    queryKey: keys.shopConsole(shopId, "products", text, available),
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      api<{ data: ShopProduct[]; nextCursor: string }>(base(shopId), { query: { q: text || undefined, available, cursor: pageParam || undefined }, signal }),
    getNextPageParam: (last) => last.nextCursor || undefined,
    enabled,
    staleTime: 15_000,
  });
}

export interface ProductPatch {
  shopPriceCents?: number;
  mrpCents?: number | null;
  stockQty?: number | null;
  isAvailable?: boolean;
  maxPerOrder?: number;
}

export function usePatchProduct(shopId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; patch: ProductPatch }) =>
      api<ShopProduct>(`${base(shopId)}/${encodeURIComponent(v.id)}`, { method: "PATCH", body: v.patch }),
    onSuccess: () => client.invalidateQueries({ queryKey: keys.shopConsole(shopId, "products") }),
  });
}
