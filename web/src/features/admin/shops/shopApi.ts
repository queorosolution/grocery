import { adminCall, useAdminMutation, useAdminQuery } from "../api";
import type { HoursDay, Settings, Shop, ShopDetail } from "../types";

const detailKey = (id: string) => ["shop", id] as const;
const listKey = ["shops"] as const;

export const useShops = () => useAdminQuery<{ data: Shop[] }>(listKey, "/shops");
export const useShopDetail = (id: string | null) => useAdminQuery<ShopDetail>(detailKey(id ?? ""), `/shops/${id}`, { enabled: id !== null });

export interface ProfileBody {
  slug: string;
  name: string;
  phone: string | null;
  addressText: string | null;
  lat: number;
  lng: number;
  logoUrl: string | null;
  themeColor: string;
}

export const useCreateShop = () =>
  useAdminMutation((body: ProfileBody) => adminCall<ShopDetail["shop"]>("POST", "/shops", body), [listKey]);

export const useUpdateShop = (id: string) =>
  useAdminMutation((body: Partial<ProfileBody> & { status?: Shop["status"] }) => adminCall("PATCH", `/shops/${id}`, body), [detailKey(id), listKey]);

export const useSetAccepting = (id: string) =>
  useAdminMutation((on: boolean) => adminCall("POST", `/shops/${id}/accepting`, { on }), [detailKey(id), listKey]);

export const useSaveSettings = (id: string) =>
  useAdminMutation((body: Settings) => adminCall("PUT", `/shops/${id}/settings`, body), [detailKey(id)]);

export const useSaveHours = (id: string) =>
  useAdminMutation((days: HoursDay[]) => adminCall("PUT", `/shops/${id}/hours`, { days }), [detailKey(id)]);

export const useCreateCartTenant = (id: string) =>
  useAdminMutation(() => adminCall("POST", `/shops/${id}/cart-tenant`), [detailKey(id)]);

export const useCartSync = (id: string) =>
  useAdminMutation(() => adminCall<{ queuedItems: number }>("POST", `/shops/${id}/cart-sync`), []);
