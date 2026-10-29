import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import { api } from "@/lib/api";
import type { LatLng } from "@/lib/geo";
import { keys } from "@/lib/keys";
import { readPin, subscribePin } from "@/lib/pinStore";
import type { HomeData } from "./types";

/** The place the shopper chose for "near you", or null. Never read on the server. */
export function usePin(): LatLng | null {
  return useSyncExternalStore(subscribePin, readPin, () => null);
}

/** Shops, deals and promos (one call, shared by the home and the search pages). */
export function useHome() {
  const pin = usePin();
  return useQuery({
    queryKey: keys.home(pin?.lat ?? null, pin?.lng ?? null),
    queryFn: ({ signal }) => api<HomeData>("/home", { query: { lat: pin?.lat, lng: pin?.lng }, signal }),
    staleTime: 60_000,
  });
}
