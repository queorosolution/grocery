import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { keys } from "@/lib/keys";
import { usePolling } from "@/lib/usePolling";
import type { DeliveryGroup, DriverStats } from "./types";

const path = (orderId: string, action: "complete" | "fail") => `/driver/deliveries/${encodeURIComponent(orderId)}/${action}`;

/** The day's list, asked again every 20 s while the page is open and visible. */
export function useDeliveries(day: string) {
  const interval = usePolling();
  return useQuery({
    queryKey: keys.driver("deliveries", day),
    queryFn: ({ signal }) => api<{ data: DeliveryGroup[] }>("/driver/deliveries", { query: { day }, signal }).then((r) => r.data),
    staleTime: 15_000,
    refetchInterval: () => interval(true),
  });
}

export function useDriverStats(from: string, to: string) {
  return useQuery({
    queryKey: keys.driver("stats", from, to),
    queryFn: ({ signal }) => api<DriverStats>("/driver/stats", { query: { from, to }, signal }),
    staleTime: 15_000,
  });
}

/** Both calls answer with the whole order, which the driver list does not hold: ask for the list and the numbers again. */
function useDeliveryCall(orderId: string, action: "complete" | "fail") {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: { version: number; code?: string; reason?: string }) => api(path(orderId, action), { method: "POST", body }),
    onSuccess: () => client.invalidateQueries({ queryKey: keys.driver() }),
    // A stale version or a missing order means the list is old too. A wrong code does not.
    onError: (err) => {
      if (err instanceof ApiError && (err.code === "STALE_ORDER" || err.code === "NOT_FOUND" || err.code === "INVALID_TRANSITION")) {
        void client.invalidateQueries({ queryKey: keys.driver("deliveries") });
      }
    },
  });
}

export const useComplete = (orderId: string) => useDeliveryCall(orderId, "complete");
export const useFail = (orderId: string) => useDeliveryCall(orderId, "fail");
