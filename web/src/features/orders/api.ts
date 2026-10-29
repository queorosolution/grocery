import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { TicketReason } from "@/lib/enums.gen";
import { newIdempotencyKey } from "@/lib/idempotencyKey";
import { keys } from "@/lib/keys";
import { usePolling } from "@/lib/usePolling";
import type { OrderDetail, OrderSummary, PaymentStart } from "./types";

const FINAL = new Set(["DELIVERED", "REJECTED", "EXPIRED", "CANCELLED", "CANCELLED_REFUND"]);
const path = (id: string) => `/orders/${encodeURIComponent(id)}`;

/** One order, asked again every 20 s while it is still moving (staleTime 0, docs/08 section 5). */
export function useOrder(id: string | null) {
  const interval = usePolling();
  return useQuery({
    queryKey: keys.order(id ?? ""),
    queryFn: ({ signal }) => api<OrderDetail>(path(id!), { signal }),
    enabled: id !== null,
    staleTime: 0,
    refetchInterval: (q) => interval(q.state.data !== undefined && !FINAL.has(q.state.data.status)),
  });
}

export function useOrders() {
  return useInfiniteQuery({
    queryKey: keys.orders(),
    initialPageParam: null as string | null,
    queryFn: ({ pageParam, signal }) =>
      api<{ data: OrderSummary[]; nextCursor: string | null }>("/orders", { query: { cursor: pageParam }, signal }),
    getNextPageParam: (last) => last.nextCursor,
    staleTime: 15_000,
  });
}

/** Every order action answers with the whole order, which goes straight into the cache (rule 4). */
function useOrderAction<V>(id: string, call: (v: V) => Promise<OrderDetail>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: call,
    onSuccess: (order) => {
      client.setQueryData(keys.order(id), order);
      void client.invalidateQueries({ queryKey: keys.orders() });
    },
  });
}

const post = (id: string, action: string, body: object) => api<OrderDetail>(`${path(id)}/${action}`, { method: "POST", body });

export const useCancelOrder = (id: string) => useOrderAction(id, (version: number) => post(id, "cancel", { version }));

export const useAcceptRevision = (id: string) =>
  useOrderAction(id, (v: { version: number; revisionNo: number }) => post(id, "accept-revision", v));

export const useApplyCoupon = (id: string) =>
  useOrderAction(id, (v: { version: number; code: string }) => post(id, "coupon", v));

export const useRemoveCoupon = (id: string) =>
  useOrderAction(id, (version: number) => api<OrderDetail>(`${path(id)}/coupon`, { method: "DELETE", body: { version } }));

/** One new key per press: the same press retried by the network gets the same answer, a new press starts a new attempt. */
export function useStartPayment(id: string) {
  return useMutation({
    mutationFn: (version: number) =>
      api<PaymentStart>(`${path(id)}/payment`, { method: "POST", body: { version }, idempotencyKey: newIdempotencyKey() }),
  });
}

export function useReportProblem(id: string) {
  return useMutation({
    mutationFn: (v: { reason: TicketReason; note: string }) =>
      api<{ id: string }>(`${path(id)}/tickets`, { method: "POST", body: v.note.trim() ? v : { reason: v.reason } }),
  });
}
