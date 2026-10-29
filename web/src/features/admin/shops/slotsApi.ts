import { adminCall, useAdminMutation, useAdminQuery } from "../api";
import type { Slot } from "../types";

const key = (shopId: string) => ["slots", shopId] as const;

export const useSlots = (shopId: string, from: string, to: string) =>
  useAdminQuery<{ data: Slot[] }>(key(shopId), `/shops/${shopId}/slots`, { query: { from, to } });

export interface NewSlot { startsAt: string; endsAt: string; cutoffAt: string; capacity: number }

// A new slot can end the "no future slot" warning, so the shop detail is asked for again too.
export const useCreateSlots = (shopId: string) =>
  useAdminMutation((slots: NewSlot[]) => adminCall("POST", `/shops/${shopId}/slots`, { slots }), [key(shopId), ["shop", shopId]]);

export const useUpdateSlot = (shopId: string) =>
  useAdminMutation((v: { id: string; capacity?: number; status?: Slot["status"] }) => {
    const { id, ...body } = v;
    return adminCall("PATCH", `/slots/${id}`, body);
  }, [key(shopId), ["shop", shopId]]);

export const useCopyWeek = (shopId: string) =>
  useAdminMutation((weekStart: string) => adminCall<{ created: number }>("POST", `/shops/${shopId}/slots/copy-week`, { weekStart }), [key(shopId), ["shop", shopId]]);
