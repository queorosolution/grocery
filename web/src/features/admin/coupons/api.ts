import { adminCall, useAdminMutation, useAdminQuery } from "../api";
import type { Coupon } from "../types";

export const useCoupons = (status: Coupon["status"] | null, shopId: string | null) =>
  useAdminQuery<{ data: Coupon[] }>(["coupons"], "/coupons", { query: { status, shopId } });

export interface NewCoupon {
  code: string;
  shopId: string | null;
  type: Coupon["type"];
  value: number;
  maxDiscountCents: number | null;
  minOrderCents: number;
  startsAt: string;
  endsAt: string;
  usageLimit: number | null;
  perUserLimit: number;
  fundedBy: Coupon["fundedBy"];
  allowLoss: boolean;
}

/** What may change later. The code, type, value, shop and who pays never do (docs/06 section 10). */
export interface CouponChange {
  status: Coupon["status"];
  startsAt: string;
  endsAt: string;
  usageLimit: number | null;
  perUserLimit: number;
  maxDiscountCents: number | null;
  minOrderCents: number;
  allowLoss: boolean;
}

export const useCreateCoupon = () => useAdminMutation((body: NewCoupon) => adminCall("POST", "/coupons", body), [["coupons"]]);
export const useUpdateCoupon = (id: string) => useAdminMutation((body: CouponChange) => adminCall("PATCH", `/coupons/${id}`, body), [["coupons"]]);
