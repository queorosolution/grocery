import { adminCall, useAdminMutation, useAdminQuery } from "../api";
import type { FeeRow, FeeRule } from "../types";

const key = ["fee-rules"] as const;

export const useFeeRules = () => useAdminQuery<{ data: FeeRow[] }>(key, "/fee-rules");

/** `shopId` null saves the platform default. */
export const useSaveRule = (shopId: string | null) =>
  useAdminMutation((rule: FeeRule) => adminCall("PUT", shopId ? `/fee-rules/shops/${shopId}` : "/fee-rules/default", rule), [key]);

export interface PreviewBody { shopId: string; distanceKm: number; weightKg: number; itemsCents: number }

export const usePreviewFee = () =>
  useAdminMutation((body: PreviewBody) => adminCall<{ feeCents: number; rule: "SHOP" | "DEFAULT" }>("POST", "/fee-rules/preview", body), []);
