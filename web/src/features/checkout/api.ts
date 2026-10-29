import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { OrderDetail } from "@/features/orders/types";
import { api } from "@/lib/api";
import type { SubstitutionPref } from "@/lib/enums.gen";
import { keys } from "@/lib/keys";
import type { Quote, Where } from "./types";

export interface BillRequest {
  shopId: string;
  cartId: string;
  where: Where;
  slotId: string | null;
  /** The cart as (item id, quantity) pairs: a changed cart is a new quote. */
  cartSignature: string;
  prefs: Record<string, SubstitutionPref>;
}

/** The body for /bills/quote and /bills. Only the choice for each line travels, never a quantity or price. */
export function billBody(r: BillRequest, itemIds: readonly string[]) {
  return {
    shopId: r.shopId,
    cartId: r.cartId,
    ...r.where,
    ...(r.slotId ? { slotId: r.slotId } : {}),
    lines: itemIds.flatMap((itemId) => (r.prefs[itemId] ? [{ itemId, subPref: r.prefs[itemId] }] : [])),
  };
}

/**
 * The bill as it would be now. Asked when the page opens and when the place, the slot or the cart
 * changes (docs/08 section 5 rule 8), never for a substitution choice: that travels with "Send".
 */
export function useQuote(request: BillRequest | null, itemIds: readonly string[]) {
  return useQuery({
    queryKey: keys.quote(request?.cartId, request?.cartSignature, request?.where, request?.slotId),
    queryFn: ({ signal }) => api<Quote>("/bills/quote", { method: "POST", body: billBody(request!, itemIds), signal }),
    enabled: request !== null,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
}

export function useSendBill() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (v: { request: BillRequest; itemIds: readonly string[]; note: string; key: string }) =>
      api<OrderDetail>("/bills", {
        method: "POST",
        idempotencyKey: v.key,
        body: { ...billBody(v.request, v.itemIds), ...(v.note.trim() ? { note: v.note.trim() } : {}) },
      }),
    onSuccess: (order) => {
      client.setQueryData(keys.order(order.id), order);
      void client.invalidateQueries({ queryKey: keys.orders() });
    },
  });
}
