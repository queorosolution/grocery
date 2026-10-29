import { useInfiniteQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys } from "@/lib/keys";
import { adminCall, useAdminMutation, useAdminQuery } from "../api";
import type { Category, Markup, Product } from "../types";

export const useCategories = () => useAdminQuery<{ data: Category[] }>(["categories"], "/categories");

export interface CategoryBody {
  slug: string;
  name: string;
  parentId: string | null;
  sort: number;
  defaultWeightG: number;
}

/** `id` set changes a category, otherwise a new one is made. */
export const useSaveCategory = (id: string | null) =>
  useAdminMutation((body: CategoryBody) => (id ? adminCall("PATCH", `/categories/${id}`, body) : adminCall("POST", "/categories", body)), [["categories"]]);

/** Master products by name, 50 a page. `q` is a word of the name, brand or keywords. */
export function useProducts(q: string) {
  return useInfiniteQuery({
    queryKey: keys.admin("products", q),
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) =>
      api<{ data: Product[]; nextCursor: string }>("/admin/products", { query: { q: q || null, cursor: pageParam || null }, signal }),
    getNextPageParam: (last) => last.nextCursor || undefined,
    staleTime: 15_000,
  });
}

export interface ProductBody {
  name: string;
  barcode: string | null;
  brand: string | null;
  sizeLabel: string | null;
  unit: Product["unit"];
  weightG: number | null;
  categoryId: string | null;
  imageUrl: string | null;
  keywords: string;
  status: Product["status"];
}

export const useSaveProduct = (id: string | null) =>
  useAdminMutation((body: ProductBody) => (id ? adminCall("PATCH", `/products/${id}`, body) : adminCall("POST", "/products", body)), [["products"], ["products", "search"]]);

export const useMarkups = (shopId: string) => useAdminQuery<{ data: Markup[] }>(["markups", shopId], `/shops/${shopId}/category-markups`);

export const useSaveMarkups = (shopId: string) =>
  useAdminMutation((markups: Markup[]) => adminCall("PUT", `/shops/${shopId}/category-markups`, { markups }), [["markups", shopId]]);

export interface OnSaleBody {
  productId: string;
  shopPriceCents: number;
  mrpCents?: number;
  stockQty?: number;
  isAvailable: boolean;
  maxPerOrder: number;
}

export const useAddShopProduct = (shopId: string) =>
  useAdminMutation((body: OnSaleBody) => adminCall("POST", `/shops/${shopId}/products`, body), []);
