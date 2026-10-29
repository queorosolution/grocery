import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { OrderStatus } from "@/lib/enums.gen";
import { keys } from "@/lib/keys";
import { usePolling } from "@/lib/usePolling";
import type { QueueRow, ShopOrder } from "./types";

const base = (shopId: string) => `/console/shops/${encodeURIComponent(shopId)}`;
const orderPath = (shopId: string, orderId: string) => `${base(shopId)}/orders/${encodeURIComponent(orderId)}`;
const FINAL: ReadonlySet<string> = new Set(["DELIVERED", "REJECTED", "EXPIRED", "CANCELLED", "CANCELLED_REFUND"]);

/** The queue for some statuses, newest first. Asked again every 20 s while the page is open and visible. */
export function useQueue(shopId: string | null, statuses: readonly OrderStatus[]) {
  const interval = usePolling();
  return useInfiniteQuery({
    queryKey: keys.shopConsole(shopId ?? "", "orders", statuses.join(",")),
    initialPageParam: null as string | null,
    queryFn: ({ pageParam, signal }) =>
      api<{ data: QueueRow[]; nextCursor: string | null }>(`${base(shopId!)}/orders`, { query: { status: statuses.join(","), cursor: pageParam }, signal }),
    getNextPageParam: (last) => last.nextCursor,
    enabled: shopId !== null,
    staleTime: 15_000,
    refetchInterval: () => interval(true),
  });
}

export function useShopOrder(shopId: string | null, orderId: string | null) {
  const interval = usePolling();
  return useQuery({
    queryKey: keys.shopConsole(shopId ?? "", "order", orderId),
    queryFn: ({ signal }) => api<ShopOrder>(orderPath(shopId!, orderId!), { signal }),
    enabled: shopId !== null && orderId !== null,
    staleTime: 0,
    refetchInterval: (q) => interval(q.state.data !== undefined && !FINAL.has(q.state.data.status)),
  });
}

/** Every order action answers with the whole order: it goes straight into the cache (docs/08 section 5 rule 4). */
function useOrderCall<V>(shopId: string, orderId: string, call: (v: V) => Promise<ShopOrder>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: call,
    onSuccess: (order) => {
      client.setQueryData(keys.shopConsole(shopId, "order", orderId), order);
      void client.invalidateQueries({ queryKey: keys.shopConsole(shopId, "orders") });
    },
  });
}

export interface LineChange {
  itemId: string;
  qty?: number;
  remove?: true;
  substitute?: { shopProductId: string; qty: number };
}

export interface ReviewBody {
  version: number;
  action: "APPROVE" | "REVISE" | "REJECT";
  reason?: string;
  slotId?: string;
  changes?: LineChange[];
}

export const useReview = (shopId: string, orderId: string) =>
  useOrderCall(shopId, orderId, (body: ReviewBody) => api<ShopOrder>(`${orderPath(shopId, orderId)}/review`, { method: "POST", body }));

export const useSetStatus = (shopId: string, orderId: string) =>
  useOrderCall(shopId, orderId, (v: { version: number; to: "PICKING" | "PACKED" | "OUT_FOR_DELIVERY" }) =>
    api<ShopOrder>(`${orderPath(shopId, orderId)}/status`, { method: "POST", body: v }));

export const useAssignDriver = (shopId: string, orderId: string) =>
  useOrderCall(shopId, orderId, (v: { version: number; driverId: string }) =>
    api<ShopOrder>(`${orderPath(shopId, orderId)}/driver`, { method: "POST", body: v }));

export function useDrivers(shopId: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.shopConsole(shopId, "drivers"),
    queryFn: () => api<{ data: { id: string; name: string }[] }>(`${base(shopId)}/drivers`).then((r) => r.data),
    enabled,
    staleTime: 60_000,
  });
}

export function useRefundRequest(shopId: string, orderId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (v: { type: "FULL" | "PARTIAL"; orderItemId?: string; reason: string }) =>
      api<{ id: string }>(`${orderPath(shopId, orderId)}/refund-requests`, { method: "POST", body: v }),
    onSuccess: () => client.invalidateQueries({ queryKey: keys.shopConsole(shopId, "order", orderId) }),
  });
}
