import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys } from "@/lib/keys";
import type { Dashboard, Payout } from "./types";

const base = (shopId: string) => `/console/shops/${encodeURIComponent(shopId)}`;

export function useDashboard(shopId: string, from: string, to: string) {
  return useQuery({
    queryKey: keys.shopConsole(shopId, "dashboard", from, to),
    queryFn: ({ signal }) => api<Dashboard>(`${base(shopId)}/dashboard`, { query: { from, to }, signal }),
    staleTime: 15_000,
  });
}

export function usePayouts(shopId: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.shopConsole(shopId, "payouts"),
    queryFn: ({ signal }) => api<{ balanceCents: number; data: Payout[] }>(`${base(shopId)}/payouts`, { signal }),
    enabled,
    staleTime: 15_000,
  });
}
