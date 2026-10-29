import { useInfiniteQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys } from "@/lib/keys";
import type { LatLng } from "@/lib/geo";
import type { SearchResult } from "./types";

export const MIN_QUERY = 2;

/** Searches as the person types: the caller passes the settled text, and a stale request is cancelled. */
export function useSearch(q: string, shop: string | null, pin: LatLng | null) {
  const text = q.trim();
  return useInfiniteQuery({
    queryKey: keys.search(text, shop, pin?.lat ?? null, pin?.lng ?? null),
    initialPageParam: null as string | null,
    queryFn: ({ pageParam, signal }) =>
      api<SearchResult>("/search", { query: { q: text, shop, lat: pin?.lat, lng: pin?.lng, cursor: pageParam }, signal }),
    getNextPageParam: (last) => last.products.nextCursor,
    enabled: text.length >= MIN_QUERY,
    staleTime: 30_000,
  });
}
